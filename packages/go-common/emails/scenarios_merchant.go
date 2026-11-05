package emails

// Emails about a merchant's application and account (merchant service). The
// guide in docs/ describes the flow: register, pending, approved or rejected,
// and suspended later.

// MerchantApplicationData fills MerchantApplicationReceived.
type MerchantApplicationData struct {
	OwnerName    string
	BusinessName string
	// ContactEmail is the business contact email from the application.
	ContactEmail string
	// SubmittedAt is the formatted time of the application (see FormatTime).
	SubmittedAt string
	// LoginURL is the absolute link to the log-in page.
	LoginURL string
}

// MerchantApplicationReceived is sent when a merchant registers (API 8.1). The
// application is pending until an administrator decides.
func MerchantApplicationReceived(d MerchantApplicationData) Email {
	return Email{
		Subject:   "We received your SwiftDrop merchant application",
		Preheader: "Your application is pending approval. We will email you when it has been reviewed.",
		Eyebrow:   "Merchant application",
		Heading:   "Your application is pending approval",
		Blocks: []Block{
			Paragraph(greeting(d.OwnerName)),
			Paragraph("Thank you for registering " + d.BusinessName + " with SwiftDrop. We review every merchant before it can create orders."),
			Notice(ToneWarning, "Status: pending approval",
				"We will email you as soon as an administrator has made a decision. You can log in meanwhile to see where your application stands."),
			Details(
				Row{Label: "Business", Value: d.BusinessName},
				Row{Label: "Contact email", Value: d.ContactEmail},
				Row{Label: "Submitted", Value: d.SubmittedAt},
			),
			Steps("What happens next",
				"An administrator reviews your business details.",
				"You receive an email with the decision.",
				"Once you are approved, you create an API key and connect your store."),
			Button("Log in to check your status", d.LoginURL),
		},
		Reason: "You are receiving this because this email address was used to register a merchant account on SwiftDrop.",
	}
}

// MerchantApprovedData fills MerchantApproved.
type MerchantApprovedData struct {
	OwnerName    string
	BusinessName string
	// ApprovedAt is the formatted time of the approval (see FormatTime).
	ApprovedAt string
	LoginURL   string
}

// MerchantApproved is sent when an administrator approves a merchant (API 8.6).
func MerchantApproved(d MerchantApprovedData) Email {
	return Email{
		Subject:   "Your SwiftDrop merchant account is approved",
		Preheader: "You can now create API keys and start sending orders.",
		Eyebrow:   "Merchant account",
		Heading:   "You are approved",
		Blocks: []Block{
			Paragraph(greeting(d.OwnerName)),
			Paragraph("Good news: " + d.BusinessName + " is now approved on SwiftDrop, and your merchant tools are unlocked."),
			Notice(ToneSuccess, "Approved", "Your account was approved on "+d.ApprovedAt+"."),
			Steps("Next steps",
				"Log in to your merchant portal.",
				"Create an API key under API keys. SwiftDrop shows the full key only once, so copy it somewhere safe.",
				"Send your first order from your store with POST /merchant/orders and your key in the X-API-Key header.",
				"Set up a webhook if you want status updates sent to your system."),
			Button("Log in to your merchant portal", d.LoginURL),
			Paragraph("Keep your API keys private. SwiftDrop will never ask you for one."),
		},
		Reason: "You are receiving this because this email address owns a merchant account on SwiftDrop.",
	}
}

// MerchantDecisionData fills MerchantRejected and MerchantSuspended.
type MerchantDecisionData struct {
	OwnerName    string
	BusinessName string
	// Reason is the reason the administrator recorded. It is shown as written.
	Reason string
}

// MerchantRejected is sent when an administrator rejects an application (API
// 8.7). The reason is required there, so it is always shown.
func MerchantRejected(d MerchantDecisionData) Email {
	return Email{
		Subject:   "Update on your SwiftDrop merchant application",
		Preheader: "We could not approve your application. Here is why.",
		Eyebrow:   "Merchant application",
		Heading:   "We could not approve your application",
		Blocks: []Block{
			Paragraph(greeting(d.OwnerName)),
			Paragraph("Thank you for applying. After review, an administrator could not approve " + d.BusinessName + " at this time."),
			Notice(ToneDanger, "Reason", d.Reason),
			Paragraph("If you can put that right, or you think this is a mistake, reply to this email or write to SwiftDrop support and we will look at it again."),
		},
		Reason: "You are receiving this because this email address was used to apply for a merchant account on SwiftDrop.",
	}
}

// MerchantSuspended is sent when an administrator suspends a merchant (API 8.8).
func MerchantSuspended(d MerchantDecisionData) Email {
	return Email{
		Subject:   "Your SwiftDrop merchant account is suspended",
		Preheader: "Your API keys are rejected while the account is suspended.",
		Eyebrow:   "Merchant account",
		Heading:   "Your merchant account is suspended",
		Blocks: []Block{
			Paragraph(greeting(d.OwnerName)),
			Paragraph("An administrator suspended " + d.BusinessName + " on SwiftDrop."),
			Notice(ToneDanger, "Reason", d.Reason),
			Paragraph("While the account is suspended, your API keys are rejected, so your store cannot create orders."),
			Paragraph("To talk about this, reply to this email or write to SwiftDrop support. An administrator can approve the account again once the problem is solved."),
		},
		Reason: "You are receiving this because this email address owns a merchant account on SwiftDrop.",
	}
}

// MerchantAPIKeyData fills MerchantAPIKeyCreated.
type MerchantAPIKeyData struct {
	OwnerName string
	// KeyName is the name the merchant gave the key.
	KeyName string
	// KeyPrefix is the visible start of the key, such as "sdk_live_4f2a9c1b".
	// Never pass the secret.
	KeyPrefix string
	// CreatedAt is the formatted time (see FormatTime).
	CreatedAt string
	// KeysURL is the absolute link to the API keys page.
	KeysURL string
}

// MerchantAPIKeyCreated is a security notice sent when a merchant creates an
// API key (API 8.10). It never contains the key.
func MerchantAPIKeyCreated(d MerchantAPIKeyData) Email {
	return Email{
		Subject:   "A new API key was created on your SwiftDrop account",
		Preheader: "If this was not you, revoke the key now.",
		Eyebrow:   "Account security",
		Heading:   "A new API key was created",
		Blocks: []Block{
			Paragraph(greeting(d.OwnerName)),
			Paragraph("A new API key was created for your merchant account."),
			Details(
				Row{Label: "Name", Value: d.KeyName},
				Row{Label: "Starts with", Value: d.KeyPrefix, IsMono: true},
				Row{Label: "Created", Value: d.CreatedAt},
			),
			Notice(ToneInfo, "We never email the key", "SwiftDrop shows a key only once, when it is created. This email has only its first characters."),
			Paragraph("If you did not create this key, revoke it now and tell SwiftDrop support."),
			Button("Review your API keys", d.KeysURL),
		},
		Reason: "You are receiving this security notice because an API key was created on your SwiftDrop merchant account.",
	}
}
