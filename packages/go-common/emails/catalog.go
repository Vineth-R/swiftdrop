package emails

import "time"

// Entry is one email SwiftDrop sends, with sample data, so every template can
// be previewed and tested without a running service.
type Entry struct {
	// ID is a stable file-name-safe name, such as "account-password-reset".
	ID    string
	Group string
	Title string
	// Trigger says when the email is sent.
	Trigger string
	Email   Email
}

// Catalog lists every email with sample data. Add a new scenario here, or the
// preview and the tests will not see it. The links are built from site.
func Catalog(site Site) []Entry {
	at := time.Date(2026, time.November, 4, 8, 35, 0, 0, time.UTC)
	when := FormatTime(at)
	const number = "SDK7M2Q9XP4"
	parcel := func(status ParcelStatus, note string) Email {
		email, err := ParcelUpdate(status, ParcelData{
			Name: "Nimali Perera", TrackingNumber: number, Origin: "Colombo", Destination: "Kandy",
			When: when, Note: note, TrackURL: site.URL("/track/" + number),
		})
		if err != nil {
			panic(err) // the statuses below are constants
		}
		return email
	}

	return []Entry{
		{
			ID: "account-password-reset", Group: "Account", Title: "Password reset",
			Trigger: "Someone asks to reset a password (POST /auth/forgot-password).",
			Email: PasswordReset(PasswordResetData{
				Name: "Nimali Perera", ValidFor: "30 minutes",
				Link: site.URL("/reset-password?token=Xk3pQ9vL2mTz7WcE5nRyB8aHdJfU4sGo1iNqZ6tVwKe"),
			}),
		},
		{
			ID: "account-password-changed", Group: "Account", Title: "Password changed",
			Trigger: "A password is changed or reset (PUT /users/me/password, POST /auth/reset-password).",
			Email: PasswordChanged(PasswordChangedData{
				Name: "Nimali Perera", When: when, ByReset: false, ResetURL: site.URL("/forgot-password"),
			}),
		},
		{
			ID: "account-welcome", Group: "Account", Title: "Welcome",
			Trigger: "A customer creates an account, by email or with Google.",
			Email:   Welcome(WelcomeData{Name: "Nimali Perera", AppURL: site.URL("/dashboard")}),
		},
		{
			ID: "account-staff-created", Group: "Account", Title: "Account created by an administrator",
			Trigger: "An administrator creates a courier, dispatcher or admin account (POST /admin/users).",
			Email: StaffAccountCreated(StaffAccountData{
				Name: "Kasun Silva", Role: "Courier", Email: "kasun.silva@example.com", LoginURL: site.URL("/login"),
			}),
		},
		{
			ID: "account-locked", Group: "Account", Title: "Account locked",
			Trigger: "Five failed log-ins lock an account.",
			Email: AccountLocked(AccountLockedData{
				Name: "Nimali Perera", LockedFor: "15 minutes", ResetURL: site.URL("/forgot-password"),
			}),
		},
		{
			ID: "account-deactivated", Group: "Account", Title: "Account deactivated",
			Trigger: "An administrator deactivates an account (PATCH /admin/users/{id}/status).",
			Email:   AccountDeactivated(AccountStatusData{Name: "Nimali Perera", LoginURL: site.URL("/login")}),
		},
		{
			ID: "account-reactivated", Group: "Account", Title: "Account reactivated",
			Trigger: "An administrator turns an account back on.",
			Email:   AccountReactivated(AccountStatusData{Name: "Nimali Perera", LoginURL: site.URL("/login")}),
		},
		{
			ID: "merchant-application-received", Group: "Merchant", Title: "Application received (pending)",
			Trigger: "A merchant registers (POST /merchants/register). The status is PENDING.",
			Email: MerchantApplicationReceived(MerchantApplicationData{
				OwnerName: "Ruwan Jayasuriya", BusinessName: "ShopZone Lanka",
				ContactEmail: "support@shopzone.lk", SubmittedAt: when, LoginURL: site.URL("/login"),
			}),
		},
		{
			ID: "merchant-approved", Group: "Merchant", Title: "Application approved",
			Trigger: "An administrator approves a merchant (POST /admin/merchants/{id}/approve).",
			Email: MerchantApproved(MerchantApprovedData{
				OwnerName: "Ruwan Jayasuriya", BusinessName: "ShopZone Lanka", ApprovedAt: when, LoginURL: site.URL("/login"),
			}),
		},
		{
			ID: "merchant-rejected", Group: "Merchant", Title: "Application rejected",
			Trigger: "An administrator rejects a merchant, with a reason (POST /admin/merchants/{id}/reject).",
			Email: MerchantRejected(MerchantDecisionData{
				OwnerName: "Ruwan Jayasuriya", BusinessName: "ShopZone Lanka",
				Reason: "The business registration number could not be verified.",
			}),
		},
		{
			ID: "merchant-suspended", Group: "Merchant", Title: "Account suspended",
			Trigger: "An administrator suspends a merchant, with a reason (POST /admin/merchants/{id}/suspend).",
			Email: MerchantSuspended(MerchantDecisionData{
				OwnerName: "Ruwan Jayasuriya", BusinessName: "ShopZone Lanka",
				Reason: "Repeated misuse of the order API.",
			}),
		},
		{
			ID: "merchant-api-key-created", Group: "Merchant", Title: "API key created",
			Trigger: "A merchant creates an API key (POST /merchants/me/keys).",
			Email: MerchantAPIKeyCreated(MerchantAPIKeyData{
				OwnerName: "Ruwan Jayasuriya", KeyName: "Production store", KeyPrefix: "sdk_live_4f2a9c1b",
				CreatedAt: when, KeysURL: site.URL("/api-keys"),
			}),
		},
		{
			ID: "parcel-created", Group: "Parcel", Title: "Parcel created",
			Trigger: "A parcel is created (parcel.created.v1).", Email: parcel(ParcelCreated, ""),
		},
		{
			ID: "parcel-picked-up", Group: "Parcel", Title: "Parcel picked up",
			Trigger: "A courier picks the parcel up (parcel.status_changed.v1).", Email: parcel(ParcelPickedUp, ""),
		},
		{
			ID: "parcel-in-transit", Group: "Parcel", Title: "Parcel in transit",
			Trigger: "The parcel is moving between hubs.", Email: parcel(ParcelInTransit, ""),
		},
		{
			ID: "parcel-out-for-delivery", Group: "Parcel", Title: "Parcel out for delivery",
			Trigger: "The courier is on the way to the recipient.", Email: parcel(ParcelOutForDelivery, ""),
		},
		{
			ID: "parcel-delivered", Group: "Parcel", Title: "Parcel delivered",
			Trigger: "The courier records the delivery, with who received it.", Email: parcel(ParcelDelivered, "Nimal Fernando"),
		},
		{
			ID: "parcel-failed", Group: "Parcel", Title: "Delivery failed",
			Trigger: "The courier records a failed delivery, with the reason.", Email: parcel(ParcelFailed, "The recipient was not at the address."),
		},
	}
}
