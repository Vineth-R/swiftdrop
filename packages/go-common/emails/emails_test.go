package emails

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

var testSite = Site{BaseURL: "https://app.swiftdrop.example", SupportEmail: "support@swiftdrop.example"}

func render(t *testing.T, e Email) mailer.Message {
	t.Helper()
	message, err := e.Message("nimali@example.com", testSite)
	if err != nil {
		t.Fatalf("Message returned error: %v", err)
	}
	return message
}

func TestCatalog_everyEmailRendersAndTheMailerAcceptsIt(t *testing.T) {
	entries := Catalog(testSite)
	if len(entries) != 18 {
		t.Fatalf("the catalog has %d emails, want 18: add a new scenario to Catalog so it is previewed and tested", len(entries))
	}

	ids, subjects := map[string]bool{}, map[string]bool{}
	mock := &mailer.Mock{}
	for _, entry := range entries {
		t.Run(entry.ID, func(t *testing.T) {
			if ids[entry.ID] {
				t.Fatalf("duplicate id %q", entry.ID)
			}
			ids[entry.ID] = true
			if !regexp.MustCompile(`^[a-z][a-z-]*$`).MatchString(entry.ID) || entry.Group == "" || entry.Title == "" || entry.Trigger == "" {
				t.Fatalf("the entry is incomplete: %+v", entry)
			}

			message := render(t, entry.Email)
			if subjects[message.Subject] {
				t.Fatalf("subject %q is used by another email", message.Subject)
			}
			subjects[message.Subject] = true
			if n := len([]rune(message.Subject)); n > 70 {
				t.Fatalf("subject is %d characters; keep it under 70 so a phone shows it whole", n)
			}
			if strings.Contains(message.Subject, "!") {
				t.Fatalf("subject %q has an exclamation mark", message.Subject)
			}
			if err := mock.Send(context.Background(), message); err != nil {
				t.Fatalf("the mailer rejects this message: %v", err)
			}
		})
	}
}

func TestEveryEmailIsAccessibleAndBranded(t *testing.T) {
	for _, entry := range Catalog(testSite) {
		t.Run(entry.ID, func(t *testing.T) {
			message := render(t, entry.Email)
			html := message.HTML

			if strings.Count(html, "<h1") != 1 {
				t.Fatalf("an email has exactly one h1, found %d", strings.Count(html, "<h1"))
			}
			for _, want := range []string{
				`<html lang="en">`,
				`name="color-scheme" content="light dark"`,
				`prefers-color-scheme: dark`,
				`cid:` + logoCID,
				`class="bg-card`,
				"<title>" + entry.Email.Subject + "</title>",
			} {
				if !strings.Contains(html, want) {
					t.Fatalf("the HTML lacks %q", want)
				}
			}
			if strings.Count(html, "<table") != strings.Count(html, `<table role="presentation"`) {
				t.Fatalf("every layout table needs role=presentation")
			}
			if strings.Contains(html, "<img") && !regexp.MustCompile(`<img [^>]*alt=`).MatchString(html) {
				t.Fatalf("an image has no alt attribute")
			}

			if entry.Email.Preheader == "" || !strings.Contains(html, "display:none") {
				t.Fatalf("the preview text is missing")
			}
			if entry.Email.Reason == "" || !strings.Contains(message.Text, entry.Email.Reason[:20]) {
				t.Fatalf("the reason the person got this email is missing")
			}
			if !strings.Contains(html, "support@swiftdrop.example") || !strings.Contains(message.Text, "support@swiftdrop.example") {
				t.Fatalf("the support address is missing from the footer")
			}
			if len(message.Inline) != 1 || message.Inline[0].ContentType != "image/png" || len(message.Inline[0].Data) < 100 {
				t.Fatalf("the logo is not embedded: %+v", message.Inline)
			}
		})
	}
}

func TestEveryLinkIsAbsoluteAndAlsoInTheTextVersion(t *testing.T) {
	href := regexp.MustCompile(`href="([^"]+)"`)
	for _, entry := range Catalog(testSite) {
		t.Run(entry.ID, func(t *testing.T) {
			message := render(t, entry.Email)
			links := href.FindAllStringSubmatch(message.HTML, -1)
			if len(links) == 0 {
				t.Fatal("no links at all")
			}
			for _, match := range links {
				link := strings.ReplaceAll(match[1], "&amp;", "&")
				if !strings.HasPrefix(link, "https://") && !strings.HasPrefix(link, "mailto:") {
					t.Fatalf("link %q is not absolute", link)
				}
			}
			for _, block := range entry.Email.Blocks {
				if block.Kind == kindButton && !strings.Contains(message.Text, block.URL) {
					t.Fatalf("the plain text version lacks the button link %q", block.URL)
				}
			}
		})
	}
}

func TestPlainTextVersion(t *testing.T) {
	for _, entry := range Catalog(testSite) {
		t.Run(entry.ID, func(t *testing.T) {
			text := render(t, entry.Email).Text

			if regexp.MustCompile(`</?[a-zA-Z][^>]*>`).MatchString(text) {
				t.Fatalf("the plain text contains HTML:\n%s", text)
			}
			if !strings.Contains(text, entry.Email.Heading) {
				t.Fatal("the plain text lacks the heading")
			}
			if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
				t.Fatal("line breaks must be CRLF")
			}
			for _, line := range strings.Split(text, "\r\n") {
				if len([]rune(line)) > textWidth+8 && !strings.Contains(line, "http") && !strings.Contains(line, "@") {
					t.Fatalf("line is %d characters, wrap it: %q", len([]rune(line)), line)
				}
			}
		})
	}
}

func TestUntrustedTextIsEscapedInHTMLAndKeptInText(t *testing.T) {
	hostile := `<script>alert(1)</script> & "quotes" <img src=x onerror=alert(2)>`

	for name, email := range map[string]Email{
		"name":     PasswordReset(PasswordResetData{Name: hostile, Link: "https://app.swiftdrop.example/reset?token=a", ValidFor: "30 minutes"}),
		"business": MerchantApproved(MerchantApprovedData{OwnerName: "Ruwan", BusinessName: hostile, ApprovedAt: "now", LoginURL: "https://app.swiftdrop.example/login"}),
		"reason":   MerchantRejected(MerchantDecisionData{OwnerName: "Ruwan", BusinessName: "ShopZone", Reason: hostile}),
		"note":     mustParcel(t, ParcelFailed, ParcelData{Name: "Nimali", TrackingNumber: "SDK1", Origin: "A", Destination: "B", Note: hostile, TrackURL: "https://app.swiftdrop.example/track/SDK1"}),
	} {
		t.Run(name, func(t *testing.T) {
			message := render(t, email)
			if strings.Contains(message.HTML, "<script") || strings.Contains(message.HTML, "<img src=x") || strings.Contains(message.HTML, "onerror=alert") && !strings.Contains(message.HTML, "&lt;img") {
				t.Fatalf("hostile text reached the HTML unescaped")
			}
			if !strings.Contains(message.HTML, "&lt;script&gt;") {
				t.Fatalf("the hostile text should appear escaped in the HTML")
			}
			if !strings.Contains(message.Text, "<script>") {
				t.Fatalf("the plain text should show the text as written")
			}
		})
	}
}

func mustParcel(t *testing.T, status ParcelStatus, data ParcelData) Email {
	t.Helper()
	email, err := ParcelUpdate(status, data)
	if err != nil {
		t.Fatalf("ParcelUpdate returned error: %v", err)
	}
	return email
}

func TestUnsafeOrBrokenEmailsAreRefused(t *testing.T) {
	good := func() Email {
		return Email{Subject: "S", Heading: "H", Blocks: []Block{Paragraph("Hello")}}
	}
	tests := map[string]func(*Email){
		"no subject":          func(e *Email) { e.Subject = " " },
		"no heading":          func(e *Email) { e.Heading = "" },
		"no blocks":           func(e *Email) { e.Blocks = nil },
		"empty paragraph":     func(e *Email) { e.Blocks = []Block{Paragraph("  ")} },
		"javascript link":     func(e *Email) { e.Blocks = []Block{Button("Go", "javascript:alert(1)")} },
		"relative link":       func(e *Email) { e.Blocks = []Block{Button("Go", "/login")} },
		"link without a host": func(e *Email) { e.Blocks = []Block{Button("Go", "https://")} },
		"data link":           func(e *Email) { e.Blocks = []Block{Button("Go", "data:text/html,hi")} },
		"button without text": func(e *Email) { e.Blocks = []Block{Button("", "https://app.swiftdrop.example")} },
		"two buttons": func(e *Email) {
			e.Blocks = []Block{Button("A", "https://a.example"), Button("B", "https://b.example")}
		},
		"details without rows": func(e *Email) { e.Blocks = []Block{Details()} },
		"steps without items":  func(e *Email) { e.Blocks = []Block{Steps("Next")} },
		"notice without text":  func(e *Email) { e.Blocks = []Block{Notice(ToneInfo, "Title", "")} },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			email := good()
			change(&email)
			if _, err := email.Message("nimali@example.com", testSite); err == nil {
				t.Fatal("Message accepted a broken email")
			}
		})
	}

	if _, err := good().Message("nimali@example.com", Site{BaseURL: "javascript:alert(1)"}); err == nil {
		t.Fatal("Message accepted an unsafe site address")
	}
	if _, err := good().Message("nimali@example.com", Site{}); err != nil {
		t.Fatalf("an email without a site address is fine, only its footer link is left out: %v", err)
	}
}

func TestSiteURL(t *testing.T) {
	for _, base := range []string{"https://app.swiftdrop.example", "https://app.swiftdrop.example/"} {
		if got := (Site{BaseURL: base}).URL("/login"); got != "https://app.swiftdrop.example/login" {
			t.Fatalf("URL = %q", got)
		}
	}
}

func TestMerchantOnboardingEmailsSayWhatTheyMustSay(t *testing.T) {
	pending := render(t, MerchantApplicationReceived(MerchantApplicationData{
		OwnerName: "Ruwan", BusinessName: "ShopZone Lanka", ContactEmail: "a@shopzone.lk", SubmittedAt: "now", LoginURL: testSite.URL("/login"),
	}))
	for _, want := range []string{"pending approval", "ShopZone Lanka", "We will email you"} {
		if !strings.Contains(pending.Text, want) {
			t.Errorf("the pending email does not say %q", want)
		}
	}

	approved := render(t, MerchantApproved(MerchantApprovedData{
		OwnerName: "Ruwan", BusinessName: "ShopZone Lanka", ApprovedAt: "now", LoginURL: testSite.URL("/login"),
	}))
	for _, want := range []string{"approved", "Next steps", "API key", "X-API-Key", testSite.URL("/login")} {
		if !strings.Contains(approved.Text, want) {
			t.Errorf("the approved email does not say %q", want)
		}
	}

	rejected := render(t, MerchantRejected(MerchantDecisionData{OwnerName: "Ruwan", BusinessName: "ShopZone", Reason: "Documents do not match."}))
	if !strings.Contains(rejected.Text, "Documents do not match.") || strings.Contains(rejected.Text, "approved on") {
		t.Errorf("the rejection email must show the reason and must not look like an approval:\n%s", rejected.Text)
	}
	suspended := render(t, MerchantSuspended(MerchantDecisionData{OwnerName: "Ruwan", BusinessName: "ShopZone", Reason: "Misuse."}))
	if !strings.Contains(suspended.Text, "API keys are rejected") || !strings.Contains(suspended.Text, "Misuse.") {
		t.Errorf("the suspension email must give the reason and the consequence:\n%s", suspended.Text)
	}
}

func TestNoEmailEverContainsASecret(t *testing.T) {
	key := render(t, MerchantAPIKeyCreated(MerchantAPIKeyData{
		OwnerName: "Ruwan", KeyName: "Store", KeyPrefix: "sdk_live_4f2a9c1b", CreatedAt: "now", KeysURL: testSite.URL("/api-keys"),
	}))
	if !strings.Contains(key.Text, "sdk_live_4f2a9c1b") || !strings.Contains(key.Text, "never email the key") {
		t.Fatalf("the key notice must show only the prefix:\n%s", key.Text)
	}

	staff := render(t, StaffAccountCreated(StaffAccountData{Name: "Kasun", Role: "Courier", Email: "k@example.com", LoginURL: testSite.URL("/login")}))
	if !strings.Contains(staff.Text, "never email passwords") || strings.Contains(strings.ToLower(staff.Text), "your password is") {
		t.Fatalf("the staff email must not contain a password:\n%s", staff.Text)
	}
}

func TestParcelUpdate(t *testing.T) {
	data := ParcelData{Name: "Nimali", TrackingNumber: "SDK7M2Q9XP4", Origin: "Colombo", Destination: "Kandy", When: "now", TrackURL: testSite.URL("/track/SDK7M2Q9XP4")}

	for _, status := range []ParcelStatus{ParcelCreated, ParcelPickedUp, ParcelInTransit, ParcelOutForDelivery, ParcelDelivered, ParcelFailed} {
		email := mustParcel(t, status, data)
		if !strings.Contains(email.Subject, "SDK7M2Q9XP4") {
			t.Errorf("%s: the subject %q lacks the tracking number", status, email.Subject)
		}
	}
	if _, err := ParcelUpdate("LOST", data); err == nil {
		t.Fatal("ParcelUpdate accepted a status that is not a parcel status")
	}

	delivered := render(t, mustParcel(t, ParcelDelivered, ParcelData{Name: "N", TrackingNumber: "SDK1", Origin: "A", Destination: "B", When: "now", Note: "Nimal Fernando", TrackURL: testSite.URL("/track/SDK1")}))
	if !strings.Contains(delivered.Text, "Received by Nimal Fernando on now.") {
		t.Errorf("the delivery email must say who received it:\n%s", delivered.Text)
	}
	failed := render(t, mustParcel(t, ParcelFailed, ParcelData{Name: "N", TrackingNumber: "SDK1", Origin: "A", Destination: "B", When: "now", TrackURL: testSite.URL("/track/SDK1")}))
	if !strings.Contains(failed.Text, "could not complete the delivery") {
		t.Errorf("a failed delivery without a reason still needs a sentence:\n%s", failed.Text)
	}
}

func TestFormatTime(t *testing.T) {
	at := time.Date(2026, time.November, 4, 8, 35, 0, 0, time.UTC)
	if got := FormatTime(at); got != "4 Nov 2026, 14:05 (Sri Lanka time)" {
		t.Fatalf("FormatTime = %q", got)
	}
	if got := FormatTime(time.Time{}); got != "" {
		t.Fatalf("a zero time must give an empty string, got %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	for duration, want := range map[time.Duration]string{
		30 * time.Minute: "30 minutes", time.Minute: "1 minute", time.Hour: "1 hour", 2 * time.Hour: "2 hours",
		90 * time.Minute: "90 minutes", 45 * time.Second: "45 seconds",
	} {
		if got := FormatDuration(duration); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", duration, got, want)
		}
	}
}

func TestGreetingAndWrap(t *testing.T) {
	if greeting("  Nimali Perera ") != "Hello Nimali Perera," || greeting("") != "Hello," {
		t.Fatal("greeting is wrong")
	}

	wrapped := wrap("one two three four five six seven eight nine ten https://app.swiftdrop.example/reset-password?token=abcdefghijklmnopqrstuvwxyz0123456789", 20)
	for _, line := range strings.Split(wrapped, "\r\n") {
		if len(line) > 20 && !strings.HasPrefix(line, "https://") {
			t.Fatalf("line %q is longer than 20 and is not a link", line)
		}
	}
	if !strings.Contains(wrapped, "https://app.swiftdrop.example/reset-password?token=abcdefghijklmnopqrstuvwxyz0123456789") {
		t.Fatal("a link must never be split")
	}
	if got := wrap("first\nsecond", 72); got != "first\r\nsecond" {
		t.Fatalf("existing line breaks must be kept, got %q", got)
	}
}
