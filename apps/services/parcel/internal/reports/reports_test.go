package reports

import (
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

var now = time.Date(2026, time.November, 5, 12, 30, 0, 0, time.UTC)

func TestParseRangeDefaults(t *testing.T) {
	issues := validation.New()
	rng, ok := ParseRange(issues, "", "", DefaultDailyDays, now)
	if !ok {
		t.Fatalf("defaults rejected: %v", issues.Issues())
	}
	if got := rng.From.Format(dateLayout); got != "2026-10-30" {
		t.Fatalf("from = %s, want 2026-10-30", got)
	}
	if got := rng.To.Format(dateLayout); got != "2026-11-05" {
		t.Fatalf("to = %s, want 2026-11-05", got)
	}
}

func TestParseRangeOnlyTo(t *testing.T) {
	issues := validation.New()
	rng, ok := ParseRange(issues, "", "2026-11-01", DefaultDailyDays, now)
	if !ok {
		t.Fatalf("rejected: %v", issues.Issues())
	}
	if got := rng.From.Format(dateLayout); got != "2026-10-26" {
		t.Fatalf("from = %s, want 2026-10-26", got)
	}
}

func TestParseRangeRejects(t *testing.T) {
	cases := map[string]struct{ from, to string }{
		"bad from format": {"01/11/2026", "2026-11-05"},
		"bad to format":   {"2026-11-01", "tomorrow"},
		"from after to":   {"2026-11-05", "2026-11-01"},
		"91 days":         {"2026-08-01", "2026-10-30"},
		"not a real date": {"2026-02-30", "2026-03-05"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			issues := validation.New()
			if _, ok := ParseRange(issues, tc.from, tc.to, DefaultDailyDays, now); ok {
				t.Fatalf("range %s to %s was accepted", tc.from, tc.to)
			}
			if issues.Empty() {
				t.Fatal("no issue was recorded")
			}
		})
	}
}

func TestParseRangeAcceptsNinetyDays(t *testing.T) {
	issues := validation.New()
	// 2026-08-02 to 2026-10-30 inclusive is exactly 90 days.
	if _, ok := ParseRange(issues, "2026-08-02", "2026-10-30", DefaultDailyDays, now); !ok {
		t.Fatalf("90 day range rejected: %v", issues.Issues())
	}
}

func TestSummarise(t *testing.T) {
	totals, success, failure := Summarise(410, 352, 21)
	if totals.InProgress != 37 {
		t.Fatalf("inProgress = %d, want 37", totals.InProgress)
	}
	if success != 94.37 || failure != 5.63 {
		t.Fatalf("rates = %v and %v, want 94.37 and 5.63", success, failure)
	}
}

func TestSummariseNothingFinished(t *testing.T) {
	totals, success, failure := Summarise(5, 0, 0)
	if totals.InProgress != 5 || success != 0 || failure != 0 {
		t.Fatalf("got %+v %v %v", totals, success, failure)
	}
}

func TestRepositoryWithoutDatabase(t *testing.T) {
	repo := NewRepository(nil)
	if _, err := repo.Daily(t.Context(), Range{}); err != ErrUnavailable {
		t.Fatalf("Daily error = %v, want ErrUnavailable", err)
	}
	if _, err := repo.Outcomes(t.Context(), Range{}); err != ErrUnavailable {
		t.Fatalf("Outcomes error = %v, want ErrUnavailable", err)
	}
}
