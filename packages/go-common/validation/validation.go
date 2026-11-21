// Package validation provides small, dependency-free helpers for collecting
// field validation issues. It does not define business rules; callers keep
// their existing messages and decide which checks apply.
package validation

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxEmailLength = 255

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Issue describes one invalid field. Field and Issue are safe to return to the
// client and never contain the submitted value.
type Issue struct {
	Field string
	Issue string
}

// Collector gathers issues in the order they are added. It is intentionally
// small: callers keep their existing validation order and messages.
type Collector struct {
	issues []Issue
}

// New returns an empty Collector.
func New() *Collector {
	return &Collector{issues: make([]Issue, 0, 4)}
}

// Add records one issue.
func (c *Collector) Add(field, issue string) {
	if c == nil {
		return
	}
	c.issues = append(c.issues, Issue{Field: field, Issue: issue})
}

// Check records issue when ok is false and returns ok.
func (c *Collector) Check(field string, ok bool, issue string) bool {
	if !ok {
		c.Add(field, issue)
	}
	return ok
}

// Required records "is required" when value is empty or whitespace-only.
func (c *Collector) Required(field, value string) bool {
	return c.Check(field, Required(value), "is required")
}

// RequiredExact records "is required" only when value is empty. Use it for
// values where whitespace may be significant, such as passwords.
func (c *Collector) RequiredExact(field, value string) bool {
	return c.Check(field, RequiredExact(value), "is required")
}

// Email records the shared email message when value is not a plain address.
func (c *Collector) Email(field, value string) bool {
	return c.Check(field, Email(value), "must be a valid email address")
}

// UUID records the shared UUID message when value is not a UUID.
func (c *Collector) UUID(field, value string) bool {
	return c.Check(field, UUID(value), "must be a UUID")
}

// Enum records issue when value is not one of allowed.
func (c *Collector) Enum(field, value, issue string, allowed ...string) bool {
	return c.Check(field, OneOf(value, allowed...), issue)
}

// Length records issue when value is shorter than min or longer than max, in
// Unicode code points.
func (c *Collector) Length(field, value string, min, max int, issue string) bool {
	length := RuneLength(value)
	return c.Check(field, length >= min && length <= max, issue)
}

// MaxLength records issue when value is longer than max, in Unicode code
// points.
func (c *Collector) MaxLength(field, value string, max int, issue string) bool {
	return c.Check(field, RuneLength(value) <= max, issue)
}

// Custom records issue when ok is false. It is for rules that stay local to a
// service, such as password policy or redirect-path checks.
func (c *Collector) Custom(field string, ok bool, issue string) bool {
	return c.Check(field, ok, issue)
}

// Issues returns a copy so callers cannot mutate the collector's state.
func (c *Collector) Issues() []Issue {
	if c == nil || len(c.issues) == 0 {
		return nil
	}
	issues := make([]Issue, len(c.issues))
	copy(issues, c.issues)
	return issues
}

// Empty reports whether no issues have been recorded.
func (c *Collector) Empty() bool {
	return c == nil || len(c.issues) == 0
}

// Len returns the number of recorded issues.
func (c *Collector) Len() int {
	if c == nil {
		return 0
	}
	return len(c.issues)
}

// Required reports whether value has non-whitespace content.
func Required(value string) bool {
	return strings.TrimSpace(value) != ""
}

// RequiredExact reports whether value is not empty.
func RequiredExact(value string) bool {
	return value != ""
}

// Email reports whether value is a plain address such as a@b.lk, without a
// display name or angle brackets.
func Email(value string) bool {
	if value == "" || len(value) > maxEmailLength {
		return false
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return false
	}
	_, domain, found := strings.Cut(value, "@")
	return found && strings.Contains(domain, ".")
}

// UUID reports whether value has the textual form of a UUID.
func UUID(value string) bool {
	return uuidPattern.MatchString(value)
}

// OneOf reports whether value exactly matches one of allowed.
func OneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

// RuneLength returns the number of Unicode code points in value.
func RuneLength(value string) int {
	return utf8.RuneCountInString(value)
}
