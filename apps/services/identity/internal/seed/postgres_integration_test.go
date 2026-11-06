//go:build integration

package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const seedTestTimeout = 120 * time.Second

func newSeedTestDatabase(t *testing.T) (context.Context, *postgres.Pool) {
	t.Helper()

	serverURL := os.Getenv("TEST_DATABASE_URL")
	if serverURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping PostgreSQL demo-seed integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedTestTimeout)
	t.Cleanup(cancel)

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_seed_test_" + seedRandomHex(8)
	quoted := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create isolated database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Logf("drop isolated database %s: %v", databaseName, err)
		}
		_ = admin.Close(cleanupCtx)
	})

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	parsed.Path = "/" + databaseName

	config := postgres.DefaultConfig()
	config.URL = parsed.String()
	config.MaxConns = 8
	config.MinConns = 0
	config.RetryAttempts = 1
	pool, err := postgres.Open(ctx, config)
	if err != nil {
		t.Fatalf("open isolated database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	for _, file := range []string{
		"infra/docker/postgres/init/01-schemas.sql",
		"migrations/identity/000001_initial_schema.up.sql",
		"migrations/identity/000002_tables.up.sql",
		"migrations/identity/000003_oauth_states_browser_binding.up.sql",
		"migrations/pricing/000001_initial_schema.up.sql",
		"migrations/pricing/000002_tables.up.sql",
	} {
		script, err := os.ReadFile(filepath.Join(seedRepositoryRoot(t), filepath.FromSlash(file)))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(script)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	return ctx, pool
}

func seedRepositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("find working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the repository root")
		}
		dir = parent
	}
}

func seedRandomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return hex.EncodeToString(buffer)
}

func seedTestHasher(t *testing.T) *password.Hasher {
	t.Helper()
	hasher, err := password.NewHasher(bcrypt.MinCost)
	if err != nil {
		t.Fatalf("NewHasher returned error: %v", err)
	}
	return hasher
}

func seedTestConfig(t *testing.T) Config {
	t.Helper()
	config, err := LoadConfig(newLoader(validValues()))
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	return config
}

type storedSeedUser struct {
	ID           string
	Email        string
	FullName     string
	Role         string
	PasswordHash string
}

func readSeedUsers(t *testing.T, ctx context.Context, pool *postgres.Pool) map[string]storedSeedUser {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT u.id::text, u.email, u.full_name, r.name, u.password_hash
		FROM identity.users u
		JOIN identity.roles r ON r.id = u.role_id
		ORDER BY u.email`)
	if err != nil {
		t.Fatalf("query seed users: %v", err)
	}
	defer rows.Close()

	users := map[string]storedSeedUser{}
	for rows.Next() {
		var user storedSeedUser
		if err := rows.Scan(&user.ID, &user.Email, &user.FullName, &user.Role, &user.PasswordHash); err != nil {
			t.Fatalf("scan seed user: %v", err)
		}
		users[user.Email] = user
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate seed users: %v", err)
	}
	return users
}

func TestRunCreatesFiveUsersAndIsIdempotent(t *testing.T) {
	ctx, pool := newSeedTestDatabase(t)
	config := seedTestConfig(t)
	hasher := seedTestHasher(t)

	first, err := Run(ctx, pool, hasher, config)
	if err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	if first.Created != 5 || first.Skipped != 0 {
		t.Fatalf("first result = %+v, want 5 created", first)
	}

	users := readSeedUsers(t, ctx, pool)
	if len(users) != 5 {
		t.Fatalf("users = %d, want 5", len(users))
	}
	wantRoles := map[string]string{
		"seed.admin@example.com":      "ADMIN",
		"seed.dispatcher@example.com": "DISPATCHER",
		"seed.courier.1@example.com":  "COURIER",
		"seed.courier.2@example.com":  "COURIER",
		"seed.customer@example.com":   "CUSTOMER",
	}
	for email, role := range wantRoles {
		user, ok := users[email]
		if !ok {
			t.Fatalf("missing user %s", email)
		}
		if user.Role != role {
			t.Fatalf("%s role = %s, want %s", email, user.Role, role)
		}
		account := findAccount(t, config, email)
		if user.PasswordHash == account.Password {
			t.Fatalf("%s stored its plaintext password", email)
		}
		if !hasher.Matches(user.PasswordHash, account.Password) {
			t.Fatalf("%s password hash does not verify", email)
		}
	}

	profileRows, err := pool.Query(ctx, `
		SELECT u.email, z.code, cp.vehicle_type, cp.vehicle_number
		FROM identity.courier_profiles cp
		JOIN identity.users u ON u.id = cp.user_id
		JOIN pricing.zones z ON z.id = cp.home_zone_id
		ORDER BY z.code`)
	if err != nil {
		t.Fatalf("query courier profiles: %v", err)
	}
	defer profileRows.Close()
	profiles := 0
	for profileRows.Next() {
		var email, code, vehicleType, vehicleNumber string
		if err := profileRows.Scan(&email, &code, &vehicleType, &vehicleNumber); err != nil {
			t.Fatalf("scan courier profile: %v", err)
		}
		profiles++
		wantCode := "CMB"
		wantVehicle := "BIKE"
		if email == "seed.courier.2@example.com" {
			wantCode = "KDY"
			wantVehicle = "VAN"
		}
		if code != wantCode || vehicleType != wantVehicle || vehicleNumber == "" {
			t.Fatalf("profile %s = zone %s vehicle %s/%s", email, code, vehicleType, vehicleNumber)
		}
	}
	if err := profileRows.Err(); err != nil {
		t.Fatalf("iterate courier profiles: %v", err)
	}
	if profiles != 2 {
		t.Fatalf("courier profiles = %d, want 2", profiles)
	}

	before := make(map[string]storedSeedUser, len(users))
	for email, user := range users {
		before[email] = user
	}
	beforeProfiles := readSeedProfiles(t, ctx, pool)

	// The second run deliberately supplies different passwords, names and
	// courier details. A skip must not update any existing account or profile.
	changed := seedTestConfig(t)
	for index := range changed.Accounts {
		changed.Accounts[index].Password += "Changed1"
		changed.Accounts[index].FullName += " Changed"
		changed.Accounts[index].VehicleType = "CHANGED"
		changed.Accounts[index].VehicleNumber = "CHANGED-00"
	}
	changed.Accounts[2].HomeZoneCode = courierTwoHomeZone
	changed.Accounts[3].HomeZoneCode = courierOneHomeZone

	second, err := Run(ctx, pool, hasher, changed)
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	if second.Created != 0 || second.Skipped != 5 {
		t.Fatalf("second result = %+v, want 0 created and 5 skipped", second)
	}

	after := readSeedUsers(t, ctx, pool)
	if len(after) != 5 {
		t.Fatalf("users after rerun = %d, want 5", len(after))
	}
	for email, old := range before {
		current, ok := after[email]
		if !ok {
			t.Fatalf("user %s disappeared", email)
		}
		if current.ID != old.ID || current.FullName != old.FullName || current.Role != old.Role || current.PasswordHash != old.PasswordHash {
			t.Fatalf("rerun changed %s: before=%+v after=%+v", email, old, current)
		}
	}
	for _, account := range changed.Accounts {
		if current := after[account.Email]; hasher.Matches(current.PasswordHash, account.Password) {
			t.Fatalf("rerun replaced %s with the changed password", account.Email)
		}
	}
	var profileCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity.courier_profiles`).Scan(&profileCount); err != nil {
		t.Fatalf("count courier profiles: %v", err)
	}
	if profileCount != 2 {
		t.Fatalf("courier profiles after rerun = %d, want 2", profileCount)
	}
	if afterProfiles := readSeedProfiles(t, ctx, pool); strings.Join(beforeProfiles, "\n") != strings.Join(afterProfiles, "\n") {
		t.Fatalf("rerun changed courier profiles: before=%v after=%v", beforeProfiles, afterProfiles)
	}
}

func readSeedProfiles(t *testing.T, ctx context.Context, pool *postgres.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT u.email || '|' || z.code || '|' || cp.vehicle_type || '|' || cp.vehicle_number
		FROM identity.courier_profiles cp
		JOIN identity.users u ON u.id = cp.user_id
		JOIN pricing.zones z ON z.id = cp.home_zone_id
		ORDER BY u.email`)
	if err != nil {
		t.Fatalf("query courier profiles: %v", err)
	}
	defer rows.Close()
	profiles := []string{}
	for rows.Next() {
		var profile string
		if err := rows.Scan(&profile); err != nil {
			t.Fatalf("scan courier profile: %v", err)
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate courier profiles: %v", err)
	}
	return profiles
}

func findAccount(t *testing.T, config Config, email string) Account {
	t.Helper()
	for _, account := range config.Accounts {
		if account.Email == email {
			return account
		}
	}
	t.Fatalf("account %s not found in config", email)
	return Account{}
}

func countSeedRows(t *testing.T, ctx context.Context, pool *postgres.Pool, table string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestRunRejectsMissingOrInactiveZoneWithoutPartialUsers(t *testing.T) {
	t.Run("unknown zone", func(t *testing.T) {
		ctx, pool := newSeedTestDatabase(t)
		config := seedTestConfig(t)
		config.Accounts[2].HomeZoneCode = "NOPE"
		_, err := Run(ctx, pool, seedTestHasher(t), config)
		if err == nil {
			t.Fatal("Run accepted an unknown zone")
		}
		if users := countSeedRows(t, ctx, pool, "identity.users"); users != 0 {
			t.Fatalf("users = %d, want 0 after zone failure", users)
		}
		if profiles := countSeedRows(t, ctx, pool, "identity.courier_profiles"); profiles != 0 {
			t.Fatalf("profiles = %d, want 0 after zone failure", profiles)
		}
	})

	t.Run("inactive zone", func(t *testing.T) {
		ctx, pool := newSeedTestDatabase(t)
		if _, err := pool.Exec(ctx, `UPDATE pricing.zones SET is_active = false WHERE code = 'CMB'`); err != nil {
			t.Fatalf("deactivate zone: %v", err)
		}
		_, err := Run(ctx, pool, seedTestHasher(t), seedTestConfig(t))
		if err == nil {
			t.Fatal("Run accepted an inactive zone")
		}
		if users := countSeedRows(t, ctx, pool, "identity.users"); users != 0 {
			t.Fatalf("users = %d, want 0 after zone failure", users)
		}
	})
}

func TestRunRollsBackOnCourierProfileFailure(t *testing.T) {
	ctx, pool := newSeedTestDatabase(t)
	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION identity.seed_test_fail_profile() RETURNS trigger AS $$
		BEGIN
			IF NEW.vehicle_number = 'SEED-VAN-01' THEN
				RAISE EXCEPTION 'forced courier profile failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER seed_test_fail_profile
			BEFORE INSERT ON identity.courier_profiles
			FOR EACH ROW EXECUTE FUNCTION identity.seed_test_fail_profile();`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	_, err := Run(ctx, pool, seedTestHasher(t), seedTestConfig(t))
	if err == nil {
		t.Fatal("Run succeeded despite the forced profile failure")
	}
	if users := countSeedRows(t, ctx, pool, "identity.users"); users != 0 {
		t.Fatalf("users = %d, want 0 after rollback", users)
	}
	if profiles := countSeedRows(t, ctx, pool, "identity.courier_profiles"); profiles != 0 {
		t.Fatalf("profiles = %d, want 0 after rollback", profiles)
	}
}

func TestRunSkipsExistingEmailWithDifferentRole(t *testing.T) {
	ctx, pool := newSeedTestDatabase(t)
	hasher := seedTestHasher(t)
	existingHash, err := hasher.Hash("ExistingPass1!")
	if err != nil {
		t.Fatalf("hash existing password: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO identity.users (email, password_hash, full_name, role_id, auth_provider)
		SELECT $1, $2, $3, ro.id, 'LOCAL'
		FROM identity.roles ro
		WHERE ro.name = 'CUSTOMER'`,
		"seed.admin@example.com", existingHash, "Existing Person"); err != nil {
		t.Fatalf("insert existing user: %v", err)
	}

	result, err := Run(ctx, pool, hasher, seedTestConfig(t))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Created != 4 || result.Skipped != 1 {
		t.Fatalf("result = %+v, want 4 created and 1 skipped", result)
	}

	users := readSeedUsers(t, ctx, pool)
	if len(users) != 5 {
		t.Fatalf("users = %d, want 5", len(users))
	}
	admin := users["seed.admin@example.com"]
	if admin.Role != "CUSTOMER" || admin.FullName != "Existing Person" || admin.PasswordHash != existingHash {
		t.Fatalf("existing account was changed: %+v", admin)
	}
	if profiles := countSeedRows(t, ctx, pool, "identity.courier_profiles"); profiles != 2 {
		t.Fatalf("courier profiles = %d, want 2", profiles)
	}
}

func TestRunConcurrentExecutionsAreSafe(t *testing.T) {
	ctx, pool := newSeedTestDatabase(t)
	config := seedTestConfig(t)
	hasher := seedTestHasher(t)

	const runs = 2
	var wait sync.WaitGroup
	errs := make(chan error, runs)
	for range runs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := Run(ctx, pool, hasher, config)
			errs <- err
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Run returned error: %v", err)
		}
	}

	if users := countSeedRows(t, ctx, pool, "identity.users"); users != 5 {
		t.Fatalf("users = %d, want 5", users)
	}
	if profiles := countSeedRows(t, ctx, pool, "identity.courier_profiles"); profiles != 2 {
		t.Fatalf("profiles = %d, want 2", profiles)
	}
}

func TestRunMissingConfigDoesNotTouchDatabase(t *testing.T) {
	ctx, pool := newSeedTestDatabase(t)
	config := seedTestConfig(t)
	config.Accounts[3].HomeZoneCode = ""

	_, err := Run(ctx, pool, seedTestHasher(t), config)
	if err == nil {
		t.Fatal("Run accepted a blank zone")
	}
	if users := countSeedRows(t, ctx, pool, "identity.users"); users != 0 {
		t.Fatalf("users = %d, want 0", users)
	}
	if strings.Contains(err.Error(), "NOPE") {
		t.Fatalf("error contains an unexpected value: %v", err)
	}
}
