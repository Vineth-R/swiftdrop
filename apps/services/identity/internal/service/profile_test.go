package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func TestService_Profile(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)

	t.Run("returns the account of the signed-in user", func(t *testing.T) {
		got, err := f.service.Profile(context.Background(), account.ID)
		if err != nil {
			t.Fatalf("Profile returned error: %v", err)
		}

		want := Profile{
			ID:           account.ID,
			Email:        "nimal@example.com",
			FullName:     "Nimal Perera",
			Phone:        "+94771234567",
			Role:         auth.RoleCustomer,
			AuthProvider: "LOCAL",
			CreatedAt:    time.Date(2026, time.November, 2, 9, 0, 0, 0, time.UTC),
		}
		if got != want {
			t.Fatalf("Profile = %+v, want %+v", got, want)
		}
	})

	t.Run("an account without a phone has an empty phone", func(t *testing.T) {
		f.store.users[account.ID].Phone = nil

		got, err := f.service.Profile(context.Background(), account.ID)
		if err != nil {
			t.Fatalf("Profile returned error: %v", err)
		}
		if got.Phone != "" {
			t.Fatalf("Phone = %q, want empty", got.Phone)
		}
	})

	t.Run("reports the sign-in provider", func(t *testing.T) {
		f.store.users[account.ID].AuthProvider = "GOOGLE"

		got, err := f.service.Profile(context.Background(), account.ID)
		if err != nil {
			t.Fatalf("Profile returned error: %v", err)
		}
		if got.AuthProvider != "GOOGLE" {
			t.Fatalf("AuthProvider = %q, want GOOGLE", got.AuthProvider)
		}
	})
}

func TestService_Profile_refusesAccountsThatAreGoneOrDisabled(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)

	t.Run("an unknown user is not found", func(t *testing.T) {
		if _, err := f.service.Profile(context.Background(), "no-such-user"); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("Profile error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("a deactivated account is refused even with a valid token", func(t *testing.T) {
		f.store.users[account.ID].IsActive = false

		_, err := f.service.Profile(context.Background(), account.ID)
		if !errors.Is(err, ErrAccountDisabled) {
			t.Fatalf("Profile error = %v, want ErrAccountDisabled", err)
		}
	})
}

func TestService_Profile_storeFailure(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	f.store.failOnOps["UserByID"] = true

	_, err := f.service.Profile(context.Background(), account.ID)
	if !errors.Is(err, errStore) {
		t.Fatalf("Profile error = %v, want the store error wrapped", err)
	}
	if errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrAccountDisabled) {
		t.Fatalf("a store failure must not look like a missing or disabled account: %v", err)
	}
}

// issuesOf returns the issues of a *ValidationError, failing the test for any
// other error.
func issuesOf(t *testing.T, err error) []Issue {
	t.Helper()
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want a *ValidationError", err)
	}
	return validation.Issues
}

func strPtr(value string) *string { return &value }

func TestService_UpdateProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("changes only the fields that were sent", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)

		got, err := f.service.UpdateProfile(ctx, account.ID, UpdateProfileInput{FullName: strPtr("Nimal P. Perera")})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		if got.FullName != "Nimal P. Perera" || got.Phone != "+94771234567" {
			t.Fatalf("got name %q phone %q, want the new name and the old phone", got.FullName, got.Phone)
		}

		got, err = f.service.UpdateProfile(ctx, account.ID, UpdateProfileInput{Phone: strPtr("+94771234000")})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		if got.FullName != "Nimal P. Perera" || got.Phone != "+94771234000" {
			t.Fatalf("got name %q phone %q, want the earlier name and the new phone", got.FullName, got.Phone)
		}
	})

	t.Run("changes both and stores them", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)

		_, err := f.service.UpdateProfile(ctx, account.ID, UpdateProfileInput{
			FullName: strPtr("Nimali Perera"),
			Phone:    strPtr("+94711112222"),
		})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		stored := f.store.users[account.ID]
		if stored.FullName != "Nimali Perera" || stored.Phone == nil || *stored.Phone != "+94711112222" {
			t.Fatalf("stored name %q phone %v, want the new values", stored.FullName, stored.Phone)
		}
	})

	t.Run("trims the name", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)

		got, err := f.service.UpdateProfile(ctx, account.ID, UpdateProfileInput{FullName: strPtr("  Nimal P. Perera \n")})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		if got.FullName != "Nimal P. Perera" {
			t.Fatalf("FullName = %q, want it trimmed", got.FullName)
		}
	})

	t.Run("leaves the email, role and provider alone", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)

		got, err := f.service.UpdateProfile(ctx, account.ID, UpdateProfileInput{FullName: strPtr("Nimali Perera")})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		if got.Email != "nimal@example.com" || got.Role != auth.RoleCustomer || got.AuthProvider != "LOCAL" {
			t.Fatalf("got %+v, want the email, role and provider unchanged", got)
		}
	})

	t.Run("a Google account may edit its name and phone too", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)
		f.store.users[account.ID].AuthProvider = "GOOGLE"
		f.store.users[account.ID].PasswordHash = nil
		f.store.users[account.ID].Phone = nil

		got, err := f.service.UpdateProfile(ctx, account.ID, UpdateProfileInput{Phone: strPtr("+94771234000")})
		if err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
		if got.Phone != "+94771234000" {
			t.Fatalf("Phone = %q, want the new phone", got.Phone)
		}
	})

	invalid := []struct {
		name  string
		input UpdateProfileInput
		want  []Issue
	}{
		{"an empty request", UpdateProfileInput{}, []Issue{{Field: "body", Issue: "must contain at least one of fullName, phone"}}},
		{"a name that is too short", UpdateProfileInput{FullName: strPtr("A")}, []Issue{{Field: "fullName", Issue: "must be 2 to 150 characters"}}},
		{"a name of only spaces", UpdateProfileInput{FullName: strPtr("     ")}, []Issue{{Field: "fullName", Issue: "must be 2 to 150 characters"}}},
		{"a name that is too long", UpdateProfileInput{FullName: strPtr(strings.Repeat("a", 151))}, []Issue{{Field: "fullName", Issue: "must be 2 to 150 characters"}}},
		{"a phone that is not E.164", UpdateProfileInput{Phone: strPtr("0771234567")}, []Issue{{Field: "phone", Issue: "must be in E.164 format, for example +94771234567"}}},
		{"an empty phone", UpdateProfileInput{Phone: strPtr("")}, []Issue{{Field: "phone", Issue: "must be in E.164 format, for example +94771234567"}}},
		{"both fields invalid", UpdateProfileInput{FullName: strPtr("A"), Phone: strPtr("x")}, []Issue{
			{Field: "fullName", Issue: "must be 2 to 150 characters"},
			{Field: "phone", Issue: "must be in E.164 format, for example +94771234567"},
		}},
	}
	for _, test := range invalid {
		t.Run("rejects "+test.name+" and stores nothing", func(t *testing.T) {
			f := newFixture(t)
			account := f.register(t)
			f.store.failOnOps["UserByID"] = true // a validation error must come before any store call

			_, err := f.service.UpdateProfile(ctx, account.ID, test.input)

			got := issuesOf(t, err)
			if len(got) != len(test.want) {
				t.Fatalf("issues = %+v, want %+v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("issue %d = %+v, want %+v", i, got[i], test.want[i])
				}
			}
			stored := f.store.users[account.ID]
			if stored.FullName != "Nimal Perera" || *stored.Phone != "+94771234567" {
				t.Fatalf("a rejected update changed the account: %+v", stored)
			}
		})
	}

	t.Run("accepts a name of 150 characters", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)

		if _, err := f.service.UpdateProfile(ctx, account.ID, UpdateProfileInput{FullName: strPtr(strings.Repeat("é", 150))}); err != nil {
			t.Fatalf("UpdateProfile returned error: %v", err)
		}
	})

	t.Run("refuses an account that is gone or deactivated", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)
		input := UpdateProfileInput{FullName: strPtr("Nimali Perera")}

		if _, err := f.service.UpdateProfile(ctx, "no-such-user", input); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("unknown user: error = %v, want ErrUserNotFound", err)
		}

		f.store.users[account.ID].IsActive = false
		if _, err := f.service.UpdateProfile(ctx, account.ID, input); !errors.Is(err, ErrAccountDisabled) {
			t.Fatalf("deactivated user: error = %v, want ErrAccountDisabled", err)
		}
		if f.store.users[account.ID].FullName != "Nimal Perera" {
			t.Fatal("a deactivated account was edited")
		}
	})

	t.Run("reports a store failure without a sentinel", func(t *testing.T) {
		for _, op := range []string{"UserByID", "UpdateProfile"} {
			f := newFixture(t)
			account := f.register(t)
			f.store.failOnOps[op] = true

			_, err := f.service.UpdateProfile(ctx, account.ID, UpdateProfileInput{FullName: strPtr("Nimali Perera")})

			if !errors.Is(err, errStore) || errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrAccountDisabled) {
				t.Fatalf("%s failing: error = %v, want the store error", op, err)
			}
		}
	})
}

func TestService_ChangePassword(t *testing.T) {
	ctx := context.Background()
	const current, next = "Str0ngPass!", "N3wStrongPass!"

	// signedInTwice registers the account and signs it in on two devices.
	signedInTwice := func(t *testing.T, f *fixture) (Account, Session, Session) {
		t.Helper()
		account := f.register(t)
		first, err := f.login(current)
		if err != nil {
			t.Fatalf("first login returned error: %v", err)
		}
		second, err := f.login(current)
		if err != nil {
			t.Fatalf("second login returned error: %v", err)
		}
		return account, first, second
	}

	t.Run("changes the password and ends every session", func(t *testing.T) {
		f := newFixture(t)
		account, first, second := signedInTwice(t, f)

		if err := f.service.ChangePassword(ctx, account.ID, ChangePasswordInput{CurrentPassword: current, NewPassword: next}); err != nil {
			t.Fatalf("ChangePassword returned error: %v", err)
		}

		if _, err := f.login(next); err != nil {
			t.Fatalf("login with the new password returned error: %v", err)
		}
		if _, err := f.login(current); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("login with the old password: error = %v, want ErrInvalidCredentials", err)
		}
		for name, session := range map[string]Session{"first": first, "second": second} {
			if _, err := f.service.Refresh(ctx, RefreshInput{RefreshToken: session.RefreshToken}); !errors.Is(err, ErrRefreshTokenInvalid) {
				t.Errorf("%s session still refreshes after the change: error = %v", name, err)
			}
		}
	})

	t.Run("stores a hash of the new password, never the password", func(t *testing.T) {
		f := newFixture(t)
		account, _, _ := signedInTwice(t, f)
		before := *f.store.users[account.ID].PasswordHash

		if err := f.service.ChangePassword(ctx, account.ID, ChangePasswordInput{CurrentPassword: current, NewPassword: next}); err != nil {
			t.Fatalf("ChangePassword returned error: %v", err)
		}

		after := *f.store.users[account.ID].PasswordHash
		if after == before || after == next || !strings.HasPrefix(after, "$2") {
			t.Fatalf("stored hash = %q, want a new bcrypt hash", after)
		}
	})

	t.Run("does not sign out anyone else", func(t *testing.T) {
		f := newFixture(t)
		account, _, _ := signedInTwice(t, f)
		other, err := f.service.Register(ctx, RegisterInput{Email: "kamal@example.com", Password: "Str0ngPass!", FullName: "Kamal Silva", Phone: "+94770000001"})
		if err != nil {
			t.Fatalf("Register returned error: %v", err)
		}
		f.store.users[other.ID].EmailVerified = true
		session, err := f.service.Login(ctx, LoginInput{Email: "kamal@example.com", Password: "Str0ngPass!"})
		if err != nil {
			t.Fatalf("Login returned error: %v", err)
		}

		if err := f.service.ChangePassword(ctx, account.ID, ChangePasswordInput{CurrentPassword: current, NewPassword: next}); err != nil {
			t.Fatalf("ChangePassword returned error: %v", err)
		}

		if _, err := f.service.Refresh(ctx, RefreshInput{RefreshToken: session.RefreshToken}); err != nil {
			t.Fatalf("another user's session ended: %v", err)
		}
		if f.store.users[other.ID].PasswordHash == nil || f.hasher.Matches(*f.store.users[other.ID].PasswordHash, next) {
			t.Fatal("another user's password changed")
		}
	})

	t.Run("clears the failed login counter and the lock", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)
		until := f.now.Add(10 * time.Minute)
		f.store.users[account.ID].FailedLoginCount, f.store.users[account.ID].LockedUntil = 5, &until

		if err := f.service.ChangePassword(ctx, account.ID, ChangePasswordInput{CurrentPassword: current, NewPassword: next}); err != nil {
			t.Fatalf("ChangePassword returned error: %v", err)
		}

		stored := f.store.users[account.ID]
		if stored.FailedLoginCount != 0 || stored.LockedUntil != nil {
			t.Fatalf("failed = %d locked until %v, want both cleared", stored.FailedLoginCount, stored.LockedUntil)
		}
	})

	t.Run("a wrong current password changes nothing and keeps the sessions", func(t *testing.T) {
		f := newFixture(t)
		account, first, _ := signedInTwice(t, f)
		before := *f.store.users[account.ID].PasswordHash

		err := f.service.ChangePassword(ctx, account.ID, ChangePasswordInput{CurrentPassword: "Wr0ngPass!", NewPassword: next})

		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("error = %v, want ErrInvalidCredentials", err)
		}
		if *f.store.users[account.ID].PasswordHash != before {
			t.Fatal("a wrong current password still changed the password")
		}
		if _, err := f.service.Refresh(ctx, RefreshInput{RefreshToken: first.RefreshToken}); err != nil {
			t.Fatalf("a failed change ended a session: %v", err)
		}
	})

	t.Run("a Google-only account has no password to change", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)
		f.store.users[account.ID].PasswordHash = nil
		f.store.users[account.ID].AuthProvider = "GOOGLE"

		err := f.service.ChangePassword(ctx, account.ID, ChangePasswordInput{CurrentPassword: current, NewPassword: next})

		if !errors.Is(err, ErrPasswordNotSet) {
			t.Fatalf("error = %v, want ErrPasswordNotSet", err)
		}
		if f.store.users[account.ID].PasswordHash != nil {
			t.Fatal("a Google-only account was given a password")
		}
	})

	invalid := []struct {
		name  string
		input ChangePasswordInput
		want  []Issue
	}{
		{"both passwords missing", ChangePasswordInput{}, []Issue{
			{Field: "currentPassword", Issue: "is required"},
			{Field: "newPassword", Issue: "is required"},
		}},
		{"the current password missing", ChangePasswordInput{NewPassword: next}, []Issue{{Field: "currentPassword", Issue: "is required"}}},
		{"a new password that is too short", ChangePasswordInput{CurrentPassword: current, NewPassword: "Sh0rt"}, []Issue{{Field: "newPassword", Issue: "must be at least 8 characters"}}},
		{"a new password without a digit", ChangePasswordInput{CurrentPassword: current, NewPassword: "NoDigitsHere"}, []Issue{{Field: "newPassword", Issue: "must contain an upper case letter, a lower case letter and a digit"}}},
		{"a new password over 72 bytes", ChangePasswordInput{CurrentPassword: current, NewPassword: "Aa1" + strings.Repeat("x", 70)}, []Issue{{Field: "newPassword", Issue: "must be at most 72 bytes"}}},
	}
	for _, test := range invalid {
		t.Run("rejects "+test.name+" before touching the account", func(t *testing.T) {
			f := newFixture(t)
			account := f.register(t)
			before := *f.store.users[account.ID].PasswordHash
			f.store.failOnOps["UserByID"] = true // validation comes before any store call

			err := f.service.ChangePassword(ctx, account.ID, test.input)

			got := issuesOf(t, err)
			if len(got) != len(test.want) {
				t.Fatalf("issues = %+v, want %+v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("issue %d = %+v, want %+v", i, got[i], test.want[i])
				}
			}
			if *f.store.users[account.ID].PasswordHash != before {
				t.Fatal("a rejected request changed the password")
			}
			for _, value := range []string{test.input.CurrentPassword, test.input.NewPassword} {
				for _, issue := range got {
					if value != "" && strings.Contains(issue.Issue, value) {
						t.Errorf("issue %q echoes a submitted password", issue.Issue)
					}
				}
			}
		})
	}

	t.Run("refuses an account that is gone or deactivated", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)
		input := ChangePasswordInput{CurrentPassword: current, NewPassword: next}

		if err := f.service.ChangePassword(ctx, "no-such-user", input); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("unknown user: error = %v, want ErrUserNotFound", err)
		}

		before := *f.store.users[account.ID].PasswordHash
		f.store.users[account.ID].IsActive = false
		if err := f.service.ChangePassword(ctx, account.ID, input); !errors.Is(err, ErrAccountDisabled) {
			t.Fatalf("deactivated user: error = %v, want ErrAccountDisabled", err)
		}
		if *f.store.users[account.ID].PasswordHash != before {
			t.Fatal("a deactivated account's password was changed")
		}
	})

	t.Run("a change that raced with another one is refused", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)
		f.store.changePasswordErr = repository.ErrPasswordMismatch

		err := f.service.ChangePassword(ctx, account.ID, ChangePasswordInput{CurrentPassword: current, NewPassword: next})

		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("an account deleted meanwhile is reported as gone", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)
		f.store.changePasswordErr = repository.ErrNotFound

		err := f.service.ChangePassword(ctx, account.ID, ChangePasswordInput{CurrentPassword: current, NewPassword: next})

		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("reports a store failure without a sentinel", func(t *testing.T) {
		for _, op := range []string{"UserByID", "ChangePassword"} {
			f := newFixture(t)
			account := f.register(t)
			f.store.failOnOps[op] = true

			err := f.service.ChangePassword(ctx, account.ID, ChangePasswordInput{CurrentPassword: current, NewPassword: next})

			if !errors.Is(err, errStore) || errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrUserNotFound) {
				t.Fatalf("%s failing: error = %v, want the store error", op, err)
			}
		}
	})
}
