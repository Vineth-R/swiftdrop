//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const (
	testZone  = "7a1e0000-0000-4000-8000-000000000001"
	otherZone = "7a1e0000-0000-4000-8000-000000000002"
)

func staffParams(email string, role auth.Role) CreateStaffParams {
	return CreateStaffParams{
		Email: email, PasswordHash: "$2a$04$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ0123",
		FullName: "Kasun Silva", Phone: "+94712223344", Role: role,
	}
}

func createCourier(t *testing.T, repo *Repository, actor, email string) AdminUser {
	t.Helper()
	params := staffParams(email, auth.RoleCourier)
	params.Courier = &CourierProfileInput{VehicleType: "BIKE", VehicleNumber: "BCD-4521", HomeZoneID: testZone}
	user, err := repo.CreateStaffUser(context.Background(), params, actor)
	if err != nil {
		t.Fatalf("CreateStaffUser returned error: %v", err)
	}
	return user
}

func TestRepository_CreateStaffUser_courierWithAuditRow(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	admin := createTestUser(t, repo, "admin@example.com")

	courier := createCourier(t, repo, admin.ID, "Kasun@SwiftDrop.lk")

	if courier.Role != auth.RoleCourier || !courier.IsActive || courier.Courier == nil {
		t.Fatalf("created = %+v", courier)
	}
	profile := courier.Courier
	if *profile.VehicleType != "BIKE" || *profile.VehicleNumber != "BCD-4521" || *profile.HomeZoneID != testZone || !profile.IsAvailable {
		t.Fatalf("profile = %+v", profile)
	}

	var provider string
	var hasPassword bool
	if err := pool.QueryRow(ctx, `SELECT auth_provider, password_hash IS NOT NULL FROM identity.users WHERE id = $1`, courier.ID).Scan(&provider, &hasPassword); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if provider != "LOCAL" || !hasPassword {
		t.Fatalf("provider=%s hasPassword=%t, want a LOCAL account with a password", provider, hasPassword)
	}

	var actor, action, entityType, entityID, details string
	if err := pool.QueryRow(ctx, `
		SELECT actor_user_id::text, action, entity_type, entity_id::text, details::text
		FROM identity.audit_logs WHERE entity_id = $1`, courier.ID,
	).Scan(&actor, &action, &entityType, &entityID, &details); err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	if actor != admin.ID || action != "USER_CREATED" || entityType != "user" || entityID != courier.ID {
		t.Fatalf("audit row = %s %s %s %s", actor, action, entityType, entityID)
	}
	var decoded map[string]any
	_ = json.Unmarshal([]byte(details), &decoded)
	if decoded["role"] != "COURIER" || len(decoded) != 1 {
		t.Fatalf("audit details = %s, want only the role", details)
	}
}

func TestRepository_CreateStaffUser_dispatcherHasNoProfile(t *testing.T) {
	repo, pool := newTestRepository(t)
	admin := createTestUser(t, repo, "admin@example.com")

	dispatcher, err := repo.CreateStaffUser(context.Background(), staffParams("dispatch@example.com", auth.RoleDispatcher), admin.ID)
	if err != nil || dispatcher.Courier != nil {
		t.Fatalf("created = %+v, err = %v", dispatcher, err)
	}
	var profiles int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.courier_profiles WHERE user_id = $1`, dispatcher.ID).Scan(&profiles)
	if profiles != 0 {
		t.Fatal("a courier profile was created for a dispatcher")
	}
}

func TestRepository_CreateStaffUser_duplicateEmailRollsBackEverything(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	admin := createTestUser(t, repo, "admin@example.com")
	createCourier(t, repo, admin.ID, "kasun@swiftdrop.lk")

	var auditBefore int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.audit_logs`).Scan(&auditBefore)

	params := staffParams("KASUN@swiftdrop.lk", auth.RoleCourier)
	params.Courier = &CourierProfileInput{VehicleType: "VAN", HomeZoneID: testZone}
	if _, err := repo.CreateStaffUser(ctx, params, admin.ID); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("error = %v, want ErrEmailTaken", err)
	}

	var auditAfter int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.audit_logs`).Scan(&auditAfter)
	if auditAfter != auditBefore {
		t.Fatal("a failed creation left an audit row")
	}
}

func TestRepository_CreateStaffUser_failedProfileLeavesNoUser(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	admin := createTestUser(t, repo, "admin@example.com")

	params := staffParams("broken@example.com", auth.RoleCourier)
	params.Courier = &CourierProfileInput{VehicleType: "this vehicle type is far too long to fit the column", HomeZoneID: testZone}
	if _, err := repo.CreateStaffUser(ctx, params, admin.ID); err == nil {
		t.Fatal("CreateStaffUser accepted an oversized vehicle type")
	}

	var users int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.users WHERE lower(email) = 'broken@example.com'`).Scan(&users)
	if users != 0 {
		t.Fatal("the user row survived a failed profile insert; the transaction did not roll back")
	}
}

func TestRepository_AdminUserByID(t *testing.T) {
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	customer := createTestUser(t, repo, "customer@example.com")

	got, err := repo.AdminUserByID(ctx, customer.ID)
	if err != nil {
		t.Fatalf("AdminUserByID returned error: %v", err)
	}
	if got.Email != "customer@example.com" || got.Role != auth.RoleCustomer || got.Courier != nil || got.LastLoginAt != nil || got.UpdatedAt.IsZero() {
		t.Fatalf("user = %+v", got)
	}

	if _, err := repo.AdminUserByID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown ID error = %v, want ErrNotFound", err)
	}
}

func TestRepository_ListAdminUsers(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	admin := createTestUser(t, repo, "admin@example.com")
	courier := createCourier(t, repo, admin.ID, "kasun@swiftdrop.lk")
	nimal := createTestUser(t, repo, "nimal.perera@example.com")
	if _, err := pool.Exec(ctx, `UPDATE identity.users SET full_name = 'Nimal Perera', is_active = false WHERE id = $1`, nimal.ID); err != nil {
		t.Fatalf("prepare data: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity.users SET created_at = $2 WHERE id = $1`, admin.ID, time.Now().Add(-3*time.Hour)); err != nil {
		t.Fatalf("prepare data: %v", err)
	}

	list := func(filter UserFilter) ([]AdminUser, int) {
		t.Helper()
		if filter.Limit == 0 {
			filter.Limit = 50
		}
		users, total, err := repo.ListAdminUsers(ctx, filter)
		if err != nil {
			t.Fatalf("ListAdminUsers(%+v) returned error: %v", filter, err)
		}
		return users, total
	}
	ids := func(users []AdminUser) []string {
		out := make([]string, len(users))
		for i, user := range users {
			out[i] = user.ID
		}
		return out
	}
	no, yes := false, true

	t.Run("everything, newest first by default sort", func(t *testing.T) {
		users, total := list(UserFilter{SortField: "createdAt", SortDesc: true})
		if total != 3 || len(users) != 3 || users[len(users)-1].ID != admin.ID {
			t.Fatalf("total = %d users = %v", total, ids(users))
		}
	})
	t.Run("by role", func(t *testing.T) {
		users, total := list(UserFilter{Role: auth.RoleCourier})
		if total != 1 || len(users) != 1 || users[0].ID != courier.ID || users[0].Courier == nil {
			t.Fatalf("total = %d users = %v", total, ids(users))
		}
	})
	t.Run("by active flag", func(t *testing.T) {
		if users, total := list(UserFilter{IsActive: &no}); total != 1 || users[0].ID != nimal.ID {
			t.Fatalf("inactive: total = %d users = %v", total, ids(users))
		}
		if _, total := list(UserFilter{IsActive: &yes}); total != 2 {
			t.Fatalf("active total = %d, want 2", total)
		}
	})
	t.Run("search matches name or email in any case", func(t *testing.T) {
		for _, term := range []string{"nimal", "NIMAL", "perera", "kasun@SWIFT", "silva"} {
			if users, _ := list(UserFilter{Search: term}); len(users) == 0 {
				t.Fatalf("search %q found nothing", term)
			}
		}
		if users, _ := list(UserFilter{Search: "nobody-by-this-name"}); len(users) != 0 {
			t.Fatal("search matched a stranger")
		}
	})
	t.Run("search treats wildcards as plain text", func(t *testing.T) {
		for _, term := range []string{"%", "_", `\`, "n%l"} {
			if users, total := list(UserFilter{Search: term}); len(users) != 0 || total != 0 {
				t.Fatalf("search %q matched %v", term, ids(users))
			}
		}
	})
	t.Run("hostile search text is just text", func(t *testing.T) {
		if users, _ := list(UserFilter{Search: "'; DROP TABLE identity.users; --"}); len(users) != 0 {
			t.Fatal("unexpected match")
		}
		if _, total := list(UserFilter{}); total != 3 {
			t.Fatal("the users table was damaged")
		}
	})
	t.Run("filters combine", func(t *testing.T) {
		if _, total := list(UserFilter{Role: auth.RoleCustomer, IsActive: &no, Search: "perera"}); total != 1 {
			t.Fatalf("total = %d, want 1", total)
		}
		if _, total := list(UserFilter{Role: auth.RoleCourier, IsActive: &no}); total != 0 {
			t.Fatalf("total = %d, want 0", total)
		}
	})
	t.Run("sorting", func(t *testing.T) {
		users, _ := list(UserFilter{SortField: "email"})
		if users[0].Email != "admin@example.com" || users[2].Email != "nimal.perera@example.com" {
			t.Fatalf("by email asc: %v", ids(users))
		}
		users, _ = list(UserFilter{SortField: "email", SortDesc: true})
		if users[0].Email != "nimal.perera@example.com" {
			t.Fatalf("by email desc: %v", ids(users))
		}
		if users, _ = list(UserFilter{SortField: "not-a-field"}); len(users) != 3 {
			t.Fatal("an unknown sort field must fall back to the default, not fail")
		}
	})
	t.Run("pagination reports the true total", func(t *testing.T) {
		first, total := list(UserFilter{SortField: "email", Limit: 2, Offset: 0})
		second, total2 := list(UserFilter{SortField: "email", Limit: 2, Offset: 2})
		if total != 3 || total2 != 3 || len(first) != 2 || len(second) != 1 {
			t.Fatalf("pages: %d + %d items, totals %d and %d", len(first), len(second), total, total2)
		}
		if beyond, total3 := list(UserFilter{Limit: 2, Offset: 20}); len(beyond) != 0 || total3 != 3 {
			t.Fatalf("a page past the end: %d items, total %d", len(beyond), total3)
		}
	})
}

func TestRepository_UpdateAdminUser(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	admin := createTestUser(t, repo, "admin@example.com")
	courier := createCourier(t, repo, admin.ID, "kasun@swiftdrop.lk")
	name, phone := "Kasun Perera", "+94770000000"

	t.Run("profile fields change, others stay", func(t *testing.T) {
		updated, err := repo.UpdateAdminUser(ctx, courier.ID, UserChange{FullName: &name, Phone: &phone}, admin.ID)
		if err != nil {
			t.Fatalf("UpdateAdminUser returned error: %v", err)
		}
		if updated.FullName != name || *updated.Phone != phone || updated.Role != auth.RoleCourier || *updated.Courier.VehicleNumber != "BCD-4521" {
			t.Fatalf("updated = %+v", updated)
		}
	})

	t.Run("a partial courier update leaves untouched fields alone", func(t *testing.T) {
		van, off := "VAN", false
		updated, err := repo.UpdateAdminUser(ctx, courier.ID, UserChange{Courier: &CourierProfileChange{VehicleType: &van, IsAvailable: &off}}, admin.ID)
		if err != nil {
			t.Fatalf("UpdateAdminUser returned error: %v", err)
		}
		p := updated.Courier
		if *p.VehicleType != "VAN" || p.IsAvailable || *p.VehicleNumber != "BCD-4521" || *p.HomeZoneID != testZone {
			t.Fatalf("profile = %+v", p)
		}
		zone := otherZone
		updated, err = repo.UpdateAdminUser(ctx, courier.ID, UserChange{Courier: &CourierProfileChange{HomeZoneID: &zone}}, admin.ID)
		if err != nil || *updated.Courier.HomeZoneID != otherZone || *updated.Courier.VehicleType != "VAN" {
			t.Fatalf("zone update: %+v, %v", updated.Courier, err)
		}
	})

	t.Run("role change is audited with before and after", func(t *testing.T) {
		dispatcher := auth.RoleDispatcher
		updated, err := repo.UpdateAdminUser(ctx, courier.ID, UserChange{Role: &dispatcher}, admin.ID)
		if err != nil || updated.Role != auth.RoleDispatcher {
			t.Fatalf("updated = %+v, err = %v", updated, err)
		}
		var details string
		if err := pool.QueryRow(ctx, `
			SELECT details::text FROM identity.audit_logs
			WHERE entity_id = $1 AND action = 'USER_UPDATED' ORDER BY created_at DESC, id LIMIT 1`, courier.ID,
		).Scan(&details); err != nil {
			t.Fatalf("read audit row: %v", err)
		}
		var decoded struct {
			Changed []string          `json:"changedFields"`
			Before  map[string]string `json:"before"`
			After   map[string]string `json:"after"`
		}
		_ = json.Unmarshal([]byte(details), &decoded)
		if decoded.Before["role"] != "COURIER" || decoded.After["role"] != "DISPATCHER" || len(decoded.Changed) != 1 || decoded.Changed[0] != "role" {
			t.Fatalf("audit details = %s", details)
		}
		for _, secret := range []string{"+9477", "Perera", "password"} {
			if strings.Contains(details, secret) {
				t.Fatalf("audit details leak %q: %s", secret, details)
			}
		}
	})

	t.Run("a customer cannot become a courier without a profile", func(t *testing.T) {
		customer := createTestUser(t, repo, "customer@example.com")
		role := auth.RoleCourier
		if _, err := repo.UpdateAdminUser(ctx, customer.ID, UserChange{Role: &role}, admin.ID); !errors.Is(err, ErrCourierProfileRequired) {
			t.Fatalf("error = %v, want ErrCourierProfileRequired", err)
		}
		reloaded, _ := repo.AdminUserByID(ctx, customer.ID)
		if reloaded.Role != auth.RoleCustomer {
			t.Fatal("the failed update changed the role anyway")
		}

		updated, err := repo.UpdateAdminUser(ctx, customer.ID, UserChange{Role: &role, Courier: &CourierProfileChange{VehicleType: strPtr("BIKE")}}, admin.ID)
		if err != nil || updated.Role != auth.RoleCourier || updated.Courier == nil || !updated.Courier.IsAvailable {
			t.Fatalf("with a profile: %+v, %v", updated, err)
		}
	})

	t.Run("profile fields are refused for a non-courier", func(t *testing.T) {
		customer := createTestUser(t, repo, "plain@example.com")
		_, err := repo.UpdateAdminUser(ctx, customer.ID, UserChange{Courier: &CourierProfileChange{VehicleType: strPtr("BIKE")}}, admin.ID)
		if !errors.Is(err, ErrCourierProfileNotAllowed) {
			t.Fatalf("error = %v, want ErrCourierProfileNotAllowed", err)
		}
		var profiles int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.courier_profiles WHERE user_id = $1`, customer.ID).Scan(&profiles)
		if profiles != 0 {
			t.Fatal("a profile row was left behind for a non-courier")
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		if _, err := repo.UpdateAdminUser(ctx, "00000000-0000-0000-0000-000000000000", UserChange{FullName: &name}, admin.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})
}

func TestRepository_SetUserActive(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	admin := createTestUser(t, repo, "admin@example.com")
	target := createTestUser(t, repo, "target@example.com")
	bystander := createTestUser(t, repo, "bystander@example.com")
	expiry := integrationNow.Add(7 * 24 * time.Hour)
	for _, userID := range []string{target.ID, target.ID, bystander.ID} {
		if _, err := repo.CreateRefreshToken(ctx, newToken(userID, "rt_"+randomHex(6), expiry)); err != nil {
			t.Fatalf("create refresh token: %v", err)
		}
	}

	if err := repo.SetUserActive(ctx, target.ID, false, admin.ID, integrationNow); err != nil {
		t.Fatalf("deactivate returned error: %v", err)
	}

	reloaded, _ := repo.UserByID(ctx, target.ID)
	if reloaded.IsActive {
		t.Fatal("the user is still active")
	}
	var targetActive, bystanderActive int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL`, target.ID).Scan(&targetActive)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL`, bystander.ID).Scan(&bystanderActive)
	if targetActive != 0 {
		t.Fatalf("%d refresh tokens survived deactivation", targetActive)
	}
	if bystanderActive != 1 {
		t.Fatal("another user's session was revoked")
	}

	var action, details string
	if err := pool.QueryRow(ctx, `SELECT action, details::text FROM identity.audit_logs WHERE entity_id = $1`, target.ID).Scan(&action, &details); err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	if action != "USER_DEACTIVATED" || !strings.Contains(details, `"isActive": false`) || !strings.Contains(details, `"isActive": true`) {
		t.Fatalf("audit = %s %s", action, details)
	}

	if err := repo.SetUserActive(ctx, target.ID, true, admin.ID, integrationNow); err != nil {
		t.Fatalf("activate returned error: %v", err)
	}
	reloaded, _ = repo.UserByID(ctx, target.ID)
	if !reloaded.IsActive {
		t.Fatal("the user is still inactive")
	}
	var actions int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.audit_logs WHERE entity_id = $1 AND action = 'USER_ACTIVATED'`, target.ID).Scan(&actions)
	if actions != 1 {
		t.Fatal("activation was not audited")
	}

	if err := repo.SetUserActive(ctx, "00000000-0000-0000-0000-000000000000", false, admin.ID, integrationNow); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown user error = %v, want ErrNotFound", err)
	}
}

func TestRepository_RotateRefreshToken_refusesADeactivatedUser(t *testing.T) {
	// A deactivated user must not be able to renew a session (M3.4 test).
	repo, _ := newTestRepository(t)
	ctx := context.Background()
	admin := createTestUser(t, repo, "admin@example.com")
	target := createTestUser(t, repo, "target@example.com")
	expiry := integrationNow.Add(time.Hour)
	if _, err := repo.CreateRefreshToken(ctx, newToken(target.ID, "rt_live", expiry)); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	if err := repo.SetUserActive(ctx, target.ID, false, admin.ID, integrationNow); err != nil {
		t.Fatalf("deactivate returned error: %v", err)
	}

	_, err := repo.RotateRefreshToken(ctx, sha256Hex("rt_live"), nextToken("rt_next", expiry), integrationNow)
	if !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("error = %v, want ErrRefreshTokenInvalid", err)
	}
}

func TestRepository_ListCouriers(t *testing.T) {
	repo, pool := newTestRepository(t)
	ctx := context.Background()
	admin := createTestUser(t, repo, "admin@example.com")
	alpha := createCourier(t, repo, admin.ID, "alpha@swiftdrop.lk")
	beta := createCourier(t, repo, admin.ID, "beta@swiftdrop.lk")
	gamma := createCourier(t, repo, admin.ID, "gamma@swiftdrop.lk")
	noProfile, err := repo.CreateStaffUser(ctx, staffParams("noprofile@swiftdrop.lk", auth.RoleDispatcher), admin.ID)
	if err != nil {
		t.Fatalf("create dispatcher: %v", err)
	}
	for statement, args := range map[string][]any{
		`UPDATE identity.users SET full_name = 'Alpha' WHERE id = $1`:                                {alpha.ID},
		`UPDATE identity.users SET full_name = 'Beta' WHERE id = $1`:                                 {beta.ID},
		`UPDATE identity.users SET full_name = 'Gamma', is_active = false WHERE id = $1`:             {gamma.ID},
		`UPDATE identity.courier_profiles SET is_available = false WHERE user_id = $1`:               {beta.ID},
		`UPDATE identity.courier_profiles SET home_zone_id = '` + otherZone + `' WHERE user_id = $1`: {beta.ID},
	} {
		if _, err := pool.Exec(ctx, statement, args...); err != nil {
			t.Fatalf("prepare data: %v", err)
		}
	}
	yes, no := true, false

	list := func(filter CourierFilter) ([]CourierListing, int) {
		t.Helper()
		filter.Limit = 50
		couriers, total, err := repo.ListCouriers(ctx, filter)
		if err != nil {
			t.Fatalf("ListCouriers returned error: %v", err)
		}
		return couriers, total
	}

	couriers, total := list(CourierFilter{})
	if total != 2 || len(couriers) != 2 || couriers[0].ID != alpha.ID || couriers[1].ID != beta.ID {
		t.Fatalf("active couriers by name: total = %d, %+v", total, couriers)
	}
	for _, courier := range couriers {
		if courier.ID == gamma.ID || courier.ID == noProfile.ID {
			t.Fatalf("listed %q, who is inactive or not a courier", courier.ID)
		}
	}
	if couriers[0].VehicleType == nil || *couriers[0].VehicleType != "BIKE" || couriers[0].HomeZoneID == nil || !couriers[0].IsAvailable {
		t.Fatalf("listing = %+v", couriers[0])
	}

	if got, total := list(CourierFilter{IsAvailable: &yes}); total != 1 || got[0].ID != alpha.ID {
		t.Fatalf("available: total = %d %+v", total, got)
	}
	if got, total := list(CourierFilter{IsAvailable: &no}); total != 1 || got[0].ID != beta.ID {
		t.Fatalf("unavailable: total = %d %+v", total, got)
	}
	if got, total := list(CourierFilter{ZoneID: otherZone}); total != 1 || got[0].ID != beta.ID {
		t.Fatalf("zone: total = %d %+v", total, got)
	}
	if _, total := list(CourierFilter{ZoneID: otherZone, IsAvailable: &yes}); total != 0 {
		t.Fatalf("zone and availability combined: total = %d, want 0", total)
	}

	// A courier whose profile row is missing is listed as unavailable.
	orphan, err := repo.CreateStaffUser(ctx, staffParams("orphan@swiftdrop.lk", auth.RoleDispatcher), admin.ID)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity.users SET role_id = (SELECT id FROM identity.roles WHERE name = 'COURIER') WHERE id = $1`, orphan.ID); err != nil {
		t.Fatalf("prepare data: %v", err)
	}
	if got, _ := list(CourierFilter{IsAvailable: &no}); len(got) != 2 {
		t.Fatalf("a courier with no profile should count as unavailable: %+v", got)
	}

	page, total, err := repo.ListCouriers(ctx, CourierFilter{Limit: 1, Offset: 1})
	if err != nil || len(page) != 1 || total != 3 {
		t.Fatalf("second page: %d items, total %d, err %v", len(page), total, err)
	}
}
