package emails

// Emails about a person's SwiftDrop account (identity service).

// PasswordResetData fills PasswordReset.
type PasswordResetData struct {
	Name string
	// Link is the absolute reset link with its token.
	Link string
	// ValidFor is how long the link works, in words: "30 minutes".
	ValidFor string
}

// PasswordReset is sent when someone asks to reset a password (API 2.16).
func PasswordReset(d PasswordResetData) Email {
	return Email{
		Subject:   "Reset your SwiftDrop password",
		Preheader: "Use this link within " + d.ValidFor + " to choose a new password.",
		Eyebrow:   "Password reset",
		Heading:   "Reset your password",
		Blocks: []Block{
			Paragraph(greeting(d.Name)),
			Paragraph("We received a request to reset the password for your SwiftDrop account. Choose a new one with the button below."),
			Button("Choose a new password", d.Link),
			Notice(ToneInfo, "This link works once",
				"It expires in "+d.ValidFor+". If you ask for another link, this one stops working."),
			Paragraph("If you did not ask for this, you can ignore this email. Your password has not changed."),
		},
		Reason: "You are receiving this because someone asked to reset the password for this email address.",
	}
}

// PasswordChangedData fills PasswordChanged.
type PasswordChangedData struct {
	Name string
	// When is the formatted time of the change (see FormatTime).
	When string
	// ByReset is true when the password was changed through a reset link and
	// false when the person changed it from their profile.
	ByReset bool
	// ResetURL is the absolute link to the forgot password page.
	ResetURL string
}

// PasswordChanged is sent after a password reset or a password change, so a
// person who did not do it finds out.
func PasswordChanged(d PasswordChangedData) Email {
	how := "From your profile"
	if d.ByReset {
		how = "With a password reset link"
	}
	return Email{
		Subject:   "Your SwiftDrop password was changed",
		Preheader: "If this was not you, reset your password now.",
		Eyebrow:   "Account security",
		Heading:   "Your password was changed",
		Blocks: []Block{
			Paragraph(greeting(d.Name)),
			Paragraph("The password for your SwiftDrop account was changed. For your security we signed you out of every device, so log in again with the new password."),
			Details(Row{Label: "When", Value: d.When}, Row{Label: "How", Value: how}),
			Notice(ToneDanger, "Was this not you?",
				"Someone else may have access to your account. Reset your password now so only you can log in, and tell support."),
			Button("Reset your password", d.ResetURL),
		},
		Reason: "You are receiving this security notice because the password of this SwiftDrop account changed.",
	}
}

// WelcomeData fills Welcome.
type WelcomeData struct {
	Name string
	// AppURL is the absolute link to open SwiftDrop (the dashboard).
	AppURL string
}

// Welcome is sent after a customer creates an account, by email or with Google.
func Welcome(d WelcomeData) Email {
	return Email{
		Subject:   "Welcome to SwiftDrop",
		Preheader: "Your account is ready. Here is how to send your first parcel.",
		Eyebrow:   "Welcome",
		Heading:   "Welcome to SwiftDrop",
		Blocks: []Block{
			Paragraph(greeting(d.Name)),
			Paragraph("Your SwiftDrop account is ready. We deliver parcels across Sri Lanka and keep you informed from pickup to the doorstep."),
			Steps("Get started",
				"Create a delivery and see the price before you confirm it.",
				"Follow every parcel with its tracking number.",
				"Keep your phone number up to date in your profile, so couriers can reach you."),
			Button("Open SwiftDrop", d.AppURL),
		},
		Reason: "You are receiving this because a SwiftDrop account was created with this email address.",
	}
}

// StaffAccountData fills StaffAccountCreated.
type StaffAccountData struct {
	Name string
	// Role in words: "Courier", "Dispatcher", "Administrator".
	Role string
	// Email is the address the person logs in with.
	Email    string
	LoginURL string
}

// StaffAccountCreated is sent when an administrator creates an account for a
// courier, a dispatcher or another administrator (API 2.11). It never contains
// the password.
func StaffAccountCreated(d StaffAccountData) Email {
	return Email{
		Subject:   "Your SwiftDrop account is ready",
		Preheader: "An administrator created a " + d.Role + " account for you.",
		Eyebrow:   "New account",
		Heading:   "Your account is ready",
		Blocks: []Block{
			Paragraph(greeting(d.Name)),
			Paragraph("An administrator created a SwiftDrop account for you."),
			Details(Row{Label: "Role", Value: d.Role}, Row{Label: "Log in with", Value: d.Email}),
			Paragraph("Use the temporary password your administrator gave you. After you log in, choose your own under Profile and Security."),
			Button("Log in", d.LoginURL),
			Notice(ToneInfo, "We never email passwords",
				"If you were not expecting this account, tell your administrator and do not log in."),
		},
		Reason: "You are receiving this because an administrator created a SwiftDrop account for this email address.",
	}
}

// AccountLockedData fills AccountLocked.
type AccountLockedData struct {
	Name string
	// LockedFor is how long the lock lasts, in words: "15 minutes".
	LockedFor string
	ResetURL  string
}

// AccountLocked is sent when repeated failed log-ins lock an account (API 2.2).
func AccountLocked(d AccountLockedData) Email {
	return Email{
		Subject:   "Your SwiftDrop account is locked for " + d.LockedFor,
		Preheader: "There were several failed log-in attempts.",
		Eyebrow:   "Account security",
		Heading:   "Your account is locked for " + d.LockedFor,
		Blocks: []Block{
			Paragraph(greeting(d.Name)),
			Paragraph("There were several failed attempts to log in to your SwiftDrop account, so we locked it for a short time to keep it safe."),
			Notice(ToneWarning, "Locked for "+d.LockedFor,
				"You can try again after that time. Nothing else about your account has changed."),
			Paragraph("If that was you, wait and try again, or reset your password now. If it was not you, reset your password so only you can log in."),
			Button("Reset your password", d.ResetURL),
		},
		Reason: "You are receiving this security notice because of failed log-in attempts on this SwiftDrop account.",
	}
}

// AccountStatusData fills AccountDeactivated and AccountReactivated.
type AccountStatusData struct {
	Name     string
	LoginURL string
}

// AccountDeactivated is sent when an administrator deactivates an account.
func AccountDeactivated(d AccountStatusData) Email {
	return Email{
		Subject:   "Your SwiftDrop account was deactivated",
		Preheader: "You can no longer log in to this account.",
		Eyebrow:   "Account",
		Heading:   "Your account was deactivated",
		Blocks: []Block{
			Paragraph(greeting(d.Name)),
			Paragraph("An administrator deactivated your SwiftDrop account, so you can no longer log in."),
			Paragraph("If you think this is a mistake, reply to this email or contact SwiftDrop support."),
		},
		Reason: "You are receiving this because the status of this SwiftDrop account changed.",
	}
}

// AccountReactivated is sent when an administrator turns an account back on.
func AccountReactivated(d AccountStatusData) Email {
	return Email{
		Subject:   "Your SwiftDrop account is active again",
		Preheader: "You can log in again.",
		Eyebrow:   "Account",
		Heading:   "Your account is active again",
		Blocks: []Block{
			Paragraph(greeting(d.Name)),
			Paragraph("An administrator turned your SwiftDrop account back on. You can log in again with your usual password."),
			Button("Log in", d.LoginURL),
		},
		Reason: "You are receiving this because the status of this SwiftDrop account changed.",
	}
}
