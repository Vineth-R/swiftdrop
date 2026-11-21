package knowledge

import (
	"strings"
	"testing"
)

func load(t *testing.T) *Base {
	t.Helper()
	base, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return base
}

func TestLoadHasApprovedSections(t *testing.T) {
	base := load(t)
	sections := base.Sections()
	if len(sections) < 20 {
		t.Fatalf("expected a populated knowledge base, got %d sections", len(sections))
	}
	for _, section := range sections {
		if section.Source == "" || section.Heading == "" || section.Content == "" {
			t.Fatalf("incomplete section: %+v", section)
		}
	}
}

func TestRetrievePricing(t *testing.T) {
	base := load(t)
	sections := base.Retrieve("How much does delivery cost from Colombo to Kandy for 2 kg?", 3)
	if len(sections) == 0 {
		t.Fatal("expected pricing knowledge")
	}
	if !strings.Contains(sections[0].Source, "pricing-faq") && !strings.Contains(sections[0].Source, "how-it-works") {
		t.Fatalf("unexpected top section: %+v", sections[0])
	}
}

func TestRetrieveTracking(t *testing.T) {
	base := load(t)
	sections := base.Retrieve("Where is my parcel? The tracking number is SDK7M2Q9XP4.", 3)
	if len(sections) == 0 {
		t.Fatal("expected tracking knowledge")
	}
}

func TestRetrieveLegalPolicyIsExplicitlyUnavailable(t *testing.T) {
	base := load(t)
	sections := base.Retrieve("Show me your terms and conditions and privacy policy", 3)
	if len(sections) == 0 {
		t.Fatal("expected policy knowledge")
	}
	var found bool
	for _, section := range sections {
		if strings.Contains(section.Source, "terms-and-conditions") || strings.Contains(section.Source, "privacy-policy") {
			found = true
			if !strings.Contains(strings.ToLower(section.Content), "does not") {
				t.Fatalf("policy section must not invent terms: %s", section.Content)
			}
		}
	}
	if !found {
		t.Fatalf("policy sections were not retrieved: %+v", sections)
	}
}

func TestRetrieveUnrelatedReturnsNothing(t *testing.T) {
	base := load(t)
	if sections := base.Retrieve("Explain quantum computing", 3); len(sections) != 0 {
		t.Fatalf("unexpected knowledge for unrelated query: %+v", sections)
	}
}

func TestFormat(t *testing.T) {
	base := load(t)
	sections := base.Retrieve("delivery status", 1)
	if len(sections) == 0 {
		t.Fatal("expected a section")
	}
	formatted := Format(sections)
	if !strings.Contains(formatted, sections[0].Source) || !strings.Contains(formatted, sections[0].Heading) {
		t.Fatalf("formatted context is missing source metadata: %q", formatted)
	}
}

func TestRetrieveLimit(t *testing.T) {
	base := load(t)
	if got := len(base.Retrieve("parcel delivery pricing tracking history", 2)); got > 2 {
		t.Fatalf("retrieved %d sections, want at most 2", got)
	}
}
