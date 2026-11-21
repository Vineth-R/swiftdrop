package validation

import (
	"reflect"
	"strings"
	"testing"
)

func TestCollectorPreservesOrderAndCopiesIssues(t *testing.T) {
	collector := New()
	collector.Add("first", "is required")
	collector.Required("second", " ")
	collector.Email("third", "not-an-email")

	want := []Issue{
		{Field: "first", Issue: "is required"},
		{Field: "second", Issue: "is required"},
		{Field: "third", Issue: "must be a valid email address"},
	}
	if got := collector.Issues(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Issues() = %+v, want %+v", got, want)
	}

	issues := collector.Issues()
	issues[0].Field = "mutated"
	if collector.Issues()[0].Field != "first" {
		t.Fatal("Issues() exposed the collector's backing array")
	}
	if collector.Len() != 3 || collector.Empty() {
		t.Fatalf("Len()/Empty() = %d/%t", collector.Len(), collector.Empty())
	}
}

func TestCollectorChecks(t *testing.T) {
	collector := New()

	if !collector.Required("name", " SwiftDrop ") {
		t.Fatal("Required() rejected a non-blank value")
	}
	if !collector.RequiredExact("password", " ") {
		t.Fatal("RequiredExact() rejected a whitespace value")
	}
	if !collector.Email("email", "person@example.com") {
		t.Fatal("Email() rejected a plain address")
	}
	if !collector.UUID("id", "3f1c7a52-0000-4000-8000-000000000001") {
		t.Fatal("UUID() rejected a UUID")
	}
	if !collector.Enum("status", "SENT", "must be one of PENDING, SENT, FAILED", "PENDING", "SENT", "FAILED") {
		t.Fatal("Enum() rejected an allowed value")
	}
	if !collector.Length("name", "ab", 2, 3, "must be 2 to 3 characters") {
		t.Fatal("Length() rejected a boundary value")
	}
	if !collector.MaxLength("name", "abc", 3, "must be at most 3 characters") {
		t.Fatal("MaxLength() rejected a boundary value")
	}
	if !collector.Custom("redirectTo", true, "must be a relative path") {
		t.Fatal("Custom() rejected a valid value")
	}
	if !collector.Empty() {
		t.Fatalf("valid values produced issues: %+v", collector.Issues())
	}

	if collector.Required("name", "") {
		t.Fatal("Required() accepted an empty value")
	}
	if collector.RequiredExact("password", "") {
		t.Fatal("RequiredExact() accepted an empty value")
	}
	if collector.Email("email", "Person <person@example.com>") {
		t.Fatal("Email() accepted a display name")
	}
	if collector.UUID("id", "not-a-uuid") {
		t.Fatal("UUID() accepted an invalid UUID")
	}
	if collector.Enum("status", "FAILED", "must be one of PENDING, SENT", "PENDING", "SENT") {
		t.Fatal("Enum() accepted an unknown value")
	}
	if collector.Length("name", "a", 2, 3, "must be 2 to 3 characters") {
		t.Fatal("Length() accepted a short value")
	}
	if collector.Length("name", "abcd", 2, 3, "must be 2 to 3 characters") {
		t.Fatal("Length() accepted a long value")
	}
	if collector.MaxLength("name", "abcd", 3, "must be at most 3 characters") {
		t.Fatal("MaxLength() accepted a long value")
	}
	if collector.Custom("redirectTo", false, "must be a relative path") {
		t.Fatal("Custom() accepted an invalid value")
	}
	if collector.Len() != 9 {
		t.Fatalf("Len() = %d, want 9", collector.Len())
	}
}

func TestPredicates(t *testing.T) {
	if !Required(" value ") || Required(" \t\n") {
		t.Fatal("Required() has the wrong semantics")
	}
	if !RequiredExact(" ") || RequiredExact("") {
		t.Fatal("RequiredExact() has the wrong semantics")
	}
	for _, value := range []string{"a@b.lk", "person@example.com"} {
		if !Email(value) {
			t.Fatalf("Email(%q) = false", value)
		}
	}
	for _, value := range []string{"", "a@b", "Person <a@b.lk>", strings.Repeat("a", 251) + "@b.lk"} {
		if Email(value) {
			t.Fatalf("Email(%q) = true", value)
		}
	}
	if !UUID("3f1c7a52-0000-4000-8000-000000000001") || UUID("3f1c7a52") {
		t.Fatal("UUID() has the wrong semantics")
	}
	if !OneOf("SENT", "PENDING", "SENT", "FAILED") || OneOf("sent", "PENDING", "SENT", "FAILED") {
		t.Fatal("OneOf() has the wrong semantics")
	}
	if got := RuneLength("aé☃"); got != 3 {
		t.Fatalf("RuneLength() = %d, want 3", got)
	}
}
