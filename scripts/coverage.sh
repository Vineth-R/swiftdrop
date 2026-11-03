#!/usr/bin/env bash
# Runs the Go tests with coverage and prints one row per service and a total.
#
#   make coverage
#
# Environment:
#   TEST_DATABASE_URL  When set, the PostgreSQL integration tests run too (build
#                      tag integration). The URL must allow CREATE DATABASE.
#   COVERAGE_MIN       Space separated service=percent gates. A service below its
#                      gate fails the run. Default: "identity=60" (task sheet M2.4).
#   COVERAGE_PROFILE   Profile written by go test. Default: coverage.out
#   COVERAGE_SUMMARY   Markdown table written for pull request descriptions.
#                      Default: coverage-summary.md
#
# The table is also appended to the GitHub job summary when GITHUB_STEP_SUMMARY
# is set, so CI shows the same numbers a developer gets locally.
set -euo pipefail

cd "$(dirname "$0")/.."

profile="${COVERAGE_PROFILE:-coverage.out}"
summary="${COVERAGE_SUMMARY:-coverage-summary.md}"
gates="${COVERAGE_MIN-identity=60}"

tags=()
database="PostgreSQL tests not included (TEST_DATABASE_URL is not set)"
if [ -n "${TEST_DATABASE_URL:-}" ]; then
  tags=(-tags=integration)
  database="PostgreSQL tests included"
fi

test_status=0
go test -race -count=1 -covermode=atomic -coverprofile="$profile" ${tags[@]+"${tags[@]}"} ./... || test_status=$?

if [ ! -s "$profile" ]; then
  printf 'error: go test wrote no coverage profile (%s).\n' "$profile" >&2
  exit "${test_status:-1}"
fi

commit="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
heading="Commit \`${commit}\` on $(date -u +%F), race detector on, ${database}."

gate_status=0
awk -v module="$(go list -m)" -v gates="$gates" -v heading="$heading" '
  NR == 1 { next } # the "mode:" line
  {
    block = $1
    file = block
    sub(/:[0-9.,]+$/, "", file)
    if (index(file, module "/") == 1) file = substr(file, length(module) + 2)

    if (match(file, /^apps\/services\/[^\/]+/)) {
      group = substr(file, 15, RLENGTH - 14)
    } else if (file ~ /^packages\/go-common\//) {
      group = "go-common"
    } else {
      next # tests/ helpers are not product code
    }

    # A block can be listed by more than one test binary: keep the best count.
    if (!(block in statements)) {
      groupOf[block] = group
      statements[block] = $2
      hits[block] = $3
      if (!(group in known)) { known[group] = 1; names[++count] = group }
    } else if ($3 > hits[block]) {
      hits[block] = $3
    }
  }
  END {
    for (i = 2; i <= count; i++) {          # alphabetical order
      for (j = i; j > 1 && names[j] < names[j - 1]; j--) {
        swap = names[j]; names[j] = names[j - 1]; names[j - 1] = swap
      }
    }
    for (block in statements) {
      g = groupOf[block]
      total[g] += statements[block]
      if (hits[block] > 0) covered[g] += statements[block]
    }

    split(gates, gateList, " ")
    for (k in gateList) {
      split(gateList[k], pair, "=")
      minimum[pair[1]] = pair[2]
    }

    print "### Go test coverage"
    print ""
    print heading
    print ""
    print "| Service | Statements | Covered | Coverage | Minimum |"
    print "| --- | ---: | ---: | ---: | --- |"
    failed = 0
    allTotal = 0
    allCovered = 0
    for (i = 1; i <= count; i++) {
      g = names[i]
      percent = total[g] ? 100 * covered[g] / total[g] : 0
      gate = "-"
      if (g in minimum) {
        if (percent + 0 >= minimum[g] + 0) {
          gate = minimum[g] "% pass"
        } else {
          gate = minimum[g] "% FAIL"
          failed = 1
        }
      }
      printf "| %s | %d | %d | %.1f%% | %s |\n", g, total[g], covered[g], percent, gate
      allTotal += total[g]
      allCovered += covered[g]
    }
    for (g in minimum) {
      if (!(g in known)) {
        printf "| %s | 0 | 0 | 0.0%% | %s%% FAIL (no coverage recorded) |\n", g, minimum[g]
        failed = 1
      }
    }
    printf "| **Total** | %d | %d | **%.1f%%** | - |\n", allTotal, allCovered, \
      (allTotal ? 100 * allCovered / allTotal : 0)
    exit failed
  }
' "$profile" | tee "$summary" || gate_status=$?

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  cat "$summary" >> "$GITHUB_STEP_SUMMARY"
fi

if [ "$test_status" -ne 0 ]; then
  printf '\nerror: go test failed (exit %s); the table above is from a partial run.\n' "$test_status" >&2
  exit "$test_status"
fi
if [ "$gate_status" -ne 0 ]; then
  printf '\nerror: a service is below its coverage minimum (COVERAGE_MIN="%s").\n' "$gates" >&2
  exit "$gate_status"
fi
