package courier

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDayBounds(t *testing.T) {
	for _, tc := range []struct{ name, date, now, want string }{
		{name: "Colombo default after midnight", now: "2026-11-06T18:30:00Z", want: "2026-11-06T18:30:00Z"},
		{name: "Colombo default before midnight", now: "2026-11-06T18:29:59Z", want: "2026-11-05T18:30:00Z"},
		{name: "explicit calendar day", date: "2026-11-07", want: "2026-11-06T18:30:00Z"},
		{name: "leap day", date: "2024-02-29", want: "2024-02-28T18:30:00Z"},
		{name: "invalid date", date: "2026-02-29"},
		{name: "PostgreSQL has no year zero", date: "0000-01-01"},
		{name: "timestamp is not date", date: "2026-11-07T00:00:00Z"},
		{name: "unpadded date", date: "2026-1-7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, _ := time.Parse(time.RFC3339, tc.now)
			start, end, err := DayBounds(tc.date, now)
			if tc.want == "" {
				if err == nil {
					t.Fatal("expected invalid date")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if start.Format(time.RFC3339) != tc.want || end.Sub(start) != 24*time.Hour {
				t.Fatalf("range = %s .. %s, want %s plus one day", start, end, tc.want)
			}
		})
	}
}

func TestRepositoryUnconfigured(t *testing.T) {
	r := NewRepository(nil)
	if _, err := r.List(context.Background(), Filter{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("List: %v", err)
	}
	if _, err := r.Detail(context.Background(), "", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Detail: %v", err)
	}
}
