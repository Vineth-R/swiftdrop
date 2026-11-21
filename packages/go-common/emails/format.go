package emails

import (
	"fmt"
	"strings"
	"time"
)

// sriLanka is UTC+5:30 all year. A fixed zone keeps the time right in a
// container image that has no time zone database.
var sriLanka = time.FixedZone("Sri Lanka", 5*3600+30*60)

// FormatTime writes a moment the way the emails show it: "4 Nov 2026, 14:05
// (Sri Lanka time)". A zero time gives an empty string.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(sriLanka).Format("2 Jan 2006, 15:04") + " (Sri Lanka time)"
}

// FormatDuration writes a length of time in words: "30 minutes", "1 hour".
func FormatDuration(d time.Duration) string {
	switch {
	case d%time.Hour == 0 && d >= time.Hour:
		return plural(int(d/time.Hour), "hour")
	case d%time.Minute == 0 && d >= time.Minute:
		return plural(int(d/time.Minute), "minute")
	default:
		return plural(int(d.Round(time.Second)/time.Second), "second")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// greeting is the first line of an email: "Hello Nimal Perera,".
func greeting(name string) string {
	if name = strings.TrimSpace(name); name == "" {
		return "Hello,"
	}
	return "Hello " + name + ","
}
