#Requires -Version 5.1
<#
.SYNOPSIS
Runs the M7.1 SQL checks in disposable PostgreSQL 16 infrastructure.
.DESCRIPTION
Checks baseline application, version 000002 DOWN/UP, and the M7.1 SQL assertions
before rollback and after reapplication. Version 000001 is never rolled back.
Requires Docker Desktop in Linux-container mode and the repository's existing
postgres:16-alpine and migrate/migrate:v4.18.3 images in the local image cache.
Uses no Compose project, .env file, host database connection, or existing volume.
Test-only trust authentication is confined to a unique internal Docker network
with no published ports. No passwords are required, loaded, or logged.
.EXAMPLE
powershell -NoProfile -File .\scripts\test-m7-database.ps1
#>
[CmdletBinding()]
param(
    [ValidateRange(5, 300)]
    [int]$StartupTimeoutSeconds = 60
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Match docker-compose.yml without resolving its environment or credentials.
$postgresImage = 'postgres:16-alpine'
$migrationImage = 'migrate/migrate:v4.18.3'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$bootstrapPath = Join-Path $repositoryRoot 'infra/docker/postgres/init/01-schemas.sql'
$sqlTestPath = Join-Path $repositoryRoot 'tests/integration/m7_database.sql'
$migrationRoot = Join-Path $repositoryRoot 'migrations'
$runId = [Guid]::NewGuid().ToString('N')
$resourcePrefix = "swiftdrop-m7-test-$runId"
$testDatabase = "swiftdrop_m7_test_$runId"
$resourceLabel = "io.swiftdrop.m7-test.run=$runId"
$networkName = "$resourcePrefix-network"
$postgresName = "$resourcePrefix-postgres"
$script:dockerCommand = $null
$cleanupNeeded = $false
$scriptExitCode = 1

function Invoke-M7Docker {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Stage,
        [Parameter(Mandatory = $true)]
        [string[]]$DockerArguments,
        [switch]$Quiet,
        [switch]$AllowFailure
    )

    # PowerShell 5.1 treats redirected native stderr as ErrorRecord objects;
    # migrate writes progress there even on success. Judge native exit codes.
    $savedErrorPreference = $ErrorActionPreference
    $PSNativeCommandUseErrorActionPreference = $false
    try {
        $ErrorActionPreference = 'Continue'
        $nativeOutput = @(& $script:dockerCommand @DockerArguments 2>&1)
        $nativeExitCode = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $savedErrorPreference
    }

    $lines = @($nativeOutput | ForEach-Object { $_.ToString() })
    if (-not $Quiet) {
        foreach ($line in $lines) {
            # Avoid echoing connection URLs if a dependency includes one in an error.
            Write-Host ($line -replace 'postgres(?:ql)?://\S+', '[test database URL redacted]')
        }
    }
    if ($nativeExitCode -ne 0 -and -not $AllowFailure) {
        $failure = [System.Exception]::new("$Stage failed (Docker exit code $nativeExitCode).")
        $failure.Data['ExitCode'] = $nativeExitCode
        throw $failure
    }
    return [PSCustomObject]@{ ExitCode = $nativeExitCode; Output = $lines }
}

function Remove-M7Resources {
    $cleanupSucceeded = $true
    # Filter by this invocation's random label, including containers left by a
    # partially failed docker run. Never prune or remove project resources.
    foreach ($resourceType in @('container', 'network')) {
        try {
            $listArguments = @($resourceType, 'ls', '--filter', "label=$resourceLabel", '--quiet')
            if ($resourceType -eq 'container') {
                $listArguments += '--all'
            }
            $lookup = Invoke-M7Docker -Stage "Find test $resourceType resources" `
                -DockerArguments $listArguments -Quiet -AllowFailure
            if ($lookup.ExitCode -ne 0) {
                throw "Cannot list this run's test $resourceType resources."
            }
            foreach ($resourceId in $lookup.Output) {
                if ([string]::IsNullOrWhiteSpace($resourceId)) {
                    continue
                }
                if ($resourceId -notmatch '^[0-9a-f]{12,64}$') {
                    $cleanupSucceeded = $false
                    Write-Host "FAIL: Unexpected output while listing test $resourceType resources."
                    continue
                }
                $removeArguments = @($resourceType, 'rm')
                if ($resourceType -eq 'container') {
                    $removeArguments += '--force'
                }
                $removeArguments += $resourceId
                $removed = Invoke-M7Docker -Stage "Remove test $resourceType" `
                    -DockerArguments $removeArguments -Quiet -AllowFailure
                if ($removed.ExitCode -ne 0) {
                    $cleanupSucceeded = $false
                    Write-Host "FAIL: Could not remove test $resourceType $resourceId."
                }
            }
        }
        catch {
            $cleanupSucceeded = $false
            Write-Host "FAIL: Test $resourceType cleanup failed for run $runId."
        }
    }
    return $cleanupSucceeded
}

function Invoke-M7Migration {
    param(
        [ValidateSet('parcel', 'notification')]
        [string]$Service,
        [ValidateSet('up', 'down')]
        [string]$Direction,
        [ValidateRange(1, 2)]
        [int]$Steps
    )

    $serviceMigrationPath = Join-Path $migrationRoot $Service
    $testDatabaseUrl = 'postgres://m7_test@m7-postgres:5432/{0}?sslmode=disable&x-migrations-table={1}_schema_migrations' `
        -f $testDatabase, $Service
    # Every migration targets the same private test network and generated DB.
    # Call migrate directly, avoiding the Makefile and Unix migration script.
    Invoke-M7Docker -Stage "$Service migrations $Direction $Steps" -DockerArguments @(
        'run', '--rm', '--pull', 'never', '--name', "$resourcePrefix-migrate-$Service",
        '--network', $networkName, '--label', $resourceLabel,
        '--mount', "type=bind,source=$serviceMigrationPath,target=/migrations,readonly",
        $migrationImage, '-path', '/migrations', '-database', $testDatabaseUrl, $Direction, "$Steps"
    )
}

function Assert-M7MigrationState {
    param(
        [ValidateRange(1, 2)]
        [int]$Version
    )

    # Use a single-quoted here-string so PowerShell preserves SQL dollar quotes.
    # Only the validated numeric version is substituted. No target comes from
    # the caller's environment or the active Compose configuration.
    $stateSql = @'
BEGIN READ ONLY;
SET LOCAL statement_timeout = '10s';
SET LOCAL lock_timeout = '2s';
DO $migration_state$
DECLARE
  expected_version BIGINT := __M7_VERSION__;
  service TEXT;
  table_name TEXT;
  expected_tables TEXT[];
  actual_version BIGINT;
  actual_dirty BOOLEAN;
BEGIN
  IF left(current_database(), char_length('swiftdrop_m7_test_')) <> 'swiftdrop_m7_test_' THEN
    RAISE EXCEPTION 'Migration checks require a disposable swiftdrop_m7_test_* database';
  END IF;
  FOREACH service IN ARRAY ARRAY['parcel', 'notification'] LOOP
    IF to_regnamespace(service) IS NULL THEN
      RAISE EXCEPTION 'FAIL migration: % schema was removed', service;
    END IF;
    RAISE NOTICE 'PASS migration: % schema remains', service;

    EXECUTE format('SELECT version, dirty FROM public.%I', service || '_schema_migrations')
      INTO STRICT actual_version, actual_dirty;
    IF actual_version IS DISTINCT FROM expected_version OR actual_dirty IS DISTINCT FROM false THEN
      RAISE EXCEPTION 'FAIL migration: % expected clean version %, got version % (dirty=%)',
        service, expected_version, actual_version, actual_dirty;
    END IF;
    RAISE NOTICE 'PASS migration: % at version %, clean', service, actual_version;

    expected_tables := CASE service
      WHEN 'parcel' THEN ARRAY['parcels', 'parcel_status_history', 'parcel_assignments',
        'delivery_proofs', 'outbox_events']
      ELSE ARRAY['notification_logs', 'processed_events']
    END;
    FOREACH table_name IN ARRAY expected_tables LOOP
      IF (to_regclass(format('%I.%I', service, table_name)) IS NULL)
          IS DISTINCT FROM (expected_version = 1) THEN
        RAISE EXCEPTION 'FAIL migration: %.% has unexpected presence at version %',
          service, table_name, expected_version;
      END IF;
      RAISE NOTICE 'PASS migration: %.% %', service, table_name,
        CASE WHEN expected_version = 1 THEN 'removed' ELSE 'present' END;
    END LOOP;
  END LOOP;
END;
$migration_state$;
ROLLBACK;
'@
    $stateSql = $stateSql.Replace('__M7_VERSION__', $Version.ToString())
    $null = Invoke-M7Docker -Stage "Verify migration state at version $Version" `
        -DockerArguments ($psqlArguments + @('--command', $stateSql))
}

try {
    foreach ($inputPath in @($bootstrapPath, $sqlTestPath)) {
        if (-not (Test-Path -LiteralPath $inputPath -PathType Leaf)) {
            throw "Required test input is missing: $inputPath"
        }
    }
    foreach ($service in @('parcel', 'notification')) {
        if (-not (Test-Path -LiteralPath (Join-Path $migrationRoot $service) -PathType Container)) {
            throw "Required migration directory is missing: $service"
        }
    }
    $script:dockerCommand = (Get-Command docker -CommandType Application -ErrorAction Stop |
        Select-Object -First 1).Source
    $engine = Invoke-M7Docker -Stage 'Check Docker Desktop' `
        -DockerArguments @('info', '--format', '{{.OSType}}') -Quiet
    if (($engine.Output -join '').Trim() -ne 'linux') {
        throw 'Docker Desktop must be running Linux containers.'
    }
    foreach ($image in @($postgresImage, $migrationImage)) {
        $available = Invoke-M7Docker -Stage 'Check cached test image' `
            -DockerArguments @('image', 'inspect', '--format', '{{.Id}}', $image) -Quiet -AllowFailure
        if ($available.ExitCode -ne 0) {
            throw "Required image is not available locally: $image. Make it available before running this test."
        }
    }

    Write-Host "Creating disposable M7 test infrastructure ($runId)."
    $cleanupNeeded = $true
    $null = Invoke-M7Docker -Stage 'Create isolated test network' -Quiet -DockerArguments @(
        'network', 'create', '--driver', 'bridge', '--internal', '--label', $resourceLabel, $networkName
    )
    # tmpfs overlays the image's PGDATA volume path, preventing even an anonymous
    # database volume. The only host mounts are existing, read-only SQL inputs.
    $null = Invoke-M7Docker -Stage 'Start disposable PostgreSQL' -Quiet -DockerArguments @(
        'run', '--detach', '--pull', 'never', '--name', $postgresName,
        '--network', $networkName, '--network-alias', 'm7-postgres', '--label', $resourceLabel,
        '--tmpfs', '/var/lib/postgresql/data:rw,size=256m',
        '--env', 'POSTGRES_USER=m7_test', '--env', "POSTGRES_DB=$testDatabase",
        '--env', 'POSTGRES_HOST_AUTH_METHOD=trust',
        '--mount', "type=bind,source=$bootstrapPath,target=/m7/bootstrap.sql,readonly",
        '--mount', "type=bind,source=$sqlTestPath,target=/m7/m7_database.sql,readonly",
        $postgresImage
    )

    Write-Host 'Waiting for disposable PostgreSQL.'
    $deadline = [DateTime]::UtcNow.AddSeconds($StartupTimeoutSeconds)
    $databaseReady = $false
    do {
        # TCP avoids the temporary socket-only server used during initdb.
        $probe = Invoke-M7Docker -Stage 'Wait for disposable PostgreSQL' -Quiet -AllowFailure `
            -DockerArguments @('exec', $postgresName, 'pg_isready', '-q', '-h', '127.0.0.1',
                '-p', '5432', '-t', '1', '-U', 'm7_test', '-d', $testDatabase)
        if ($probe.ExitCode -eq 0) {
            $databaseReady = $true
            break
        }
        Start-Sleep -Milliseconds 500
    } while ([DateTime]::UtcNow -lt $deadline)
    if (-not $databaseReady) {
        throw "Disposable PostgreSQL did not become ready within $StartupTimeoutSeconds seconds."
    }

    $psqlArguments = @('exec', $postgresName, 'psql', '-X', '--no-password',
        '--host', '127.0.0.1', '--port', '5432', '--username', 'm7_test',
        '--dbname', $testDatabase, '--set', 'ON_ERROR_STOP=1')
    Write-Host 'Applying the existing database bootstrap.'
    $null = Invoke-M7Docker -Stage 'Apply database bootstrap' `
        -DockerArguments ($psqlArguments + @('--file', '/m7/bootstrap.sql'))

    foreach ($service in @('parcel', 'notification')) {
        Write-Host "Applying existing $service baseline migrations (up 2)."
        $null = Invoke-M7Migration -Service $service -Direction up -Steps 2
    }
    Assert-M7MigrationState -Version 2

    Write-Host 'Running M7.1 database assertions before rollback.'
    $null = Invoke-M7Docker -Stage 'Run M7.1 SQL tests' `
        -DockerArguments ($psqlArguments + @('--file', '/m7/m7_database.sql'))

    foreach ($service in @('notification', 'parcel')) {
        Write-Host "Rolling back $service version 000002 only (down 1)."
        $null = Invoke-M7Migration -Service $service -Direction down -Steps 1
    }
    Assert-M7MigrationState -Version 1

    foreach ($service in @('parcel', 'notification')) {
        Write-Host "Reapplying $service version 000002 (up 1)."
        $null = Invoke-M7Migration -Service $service -Direction up -Steps 1
    }
    Assert-M7MigrationState -Version 2
    Write-Host 'Running M7.1 database assertions after reapplication.'
    $null = Invoke-M7Docker -Stage 'Rerun M7.1 SQL tests after reapplication' `
        -DockerArguments ($psqlArguments + @('--file', '/m7/m7_database.sql'))
    $scriptExitCode = 0
}
catch {
    if ($_.Exception.Data.Contains('ExitCode')) {
        $scriptExitCode = [int]$_.Exception.Data['ExitCode']
    }
    Write-Host ("FAIL: {0}" -f $_.Exception.Message)
}
finally {
    if ($cleanupNeeded) {
        Write-Host 'Cleaning up this run''s disposable containers and network.'
        $cleanupComplete = Remove-M7Resources
        if (-not $cleanupComplete -and $scriptExitCode -eq 0) {
            $scriptExitCode = 1
        }
    }
}

if ($scriptExitCode -eq 0) {
    Write-Host 'M7.1 database verification passed; disposable infrastructure removed.'
}
exit $scriptExitCode
