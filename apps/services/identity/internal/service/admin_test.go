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

const (
	adminID   = "3f1c7a52-0000-4000-8000-0000000000a1"
	targetID  = "3f1c7a52-0000-4000-8000-0000000000b2"
	zoneIDStr = "7a1e0000-0000-4000-8000-000000000001"
)

// fakeAdminStore records the last call and returns canned results.
type fakeAdminStore struct {
	users   map[string]repository.AdminUser
	listed  []repository.AdminUser
	total   int
	err     error // returned by every write and list
	findErr error

	listFilter    repository.UserFilter
	created       repository.CreateStaffParams
	createActor   string
	updatedID     string
	updatedChange repository.UserChange
	updateActor   string
	statusID      string
	statusActive  *bool
	statusActor   string
	courierFilter repository.CourierFilter
	writes        int
}

func newFakeAdminStore() *fakeAdminStore {
	return &fakeAdminStore{users: map[string]repository.AdminUser{
		targetID: {ID: targetID, Email: "t@example.com", FullName: "Target", Role: auth.RoleCustomer, IsActive: true},
		adminID:  {ID: adminID, Email: "a@example.com", FullName: "Admin", Role: auth.RoleAdmin, IsActive: true},
	}}
}

func (f *fakeAdminStore) ListAdminUsers(_ context.Context, filter repository.UserFilter) ([]repository.AdminUser, int, error) {
	f.listFilter = filter
	return f.listed, f.total, f.err
}

func (f *fakeAdminStore) AdminUserByID(_ context.Context, id string) (repository.AdminUser, error) {
	if f.findErr != nil {
		return repository.AdminUser{}, f.findErr
	}
	user, ok := f.users[id]
	if !ok {
		return repository.AdminUser{}, repository.ErrNotFound
	}
	return user, nil
}

func (f *fakeAdminStore) CreateStaffUser(_ context.Context, params repository.CreateStaffParams, actor string) (repository.AdminUser, error) {
	f.writes++
	f.created, f.createActor = params, actor
	if f.err != nil {
		return repository.AdminUser{}, f.err
	}
	return repository.AdminUser{ID: "new-id", Email: params.Email, FullName: params.FullName, Role: params.Role, IsActive: true}, nil
}

func (f *fakeAdminStore) UpdateAdminUser(_ context.Context, id string, change repository.UserChange, actor string) (repository.AdminUser, error) {
	f.writes++
	f.updatedID, f.updatedChange, f.updateActor = id, change, actor
	if f.err != nil {
		return repository.AdminUser{}, f.err
	}
	return repository.AdminUser{ID: id}, nil
}

func (f *fakeAdminStore) SetUserActive(_ context.Context, id string, active bool, actor string, _ time.Time) error {
	f.writes++
	f.statusID, f.statusActive, f.statusActor = id, &active, actor
	return f.err
}

func (f *fakeAdminStore) ListCouriers(_ context.Context, filter repository.CourierFilter) ([]repository.CourierListing, int, error) {
	f.courierFilter = filter
	return nil, f.total, f.err
}

func newAdminFixture(t *testing.T) (*AdminUsers, *fakeAdminStore, *fixture) {
	t.Helper()
	base := newFixture(t)
	store := newFakeAdminStore()
	admin, err := NewAdminUsers(base.service, store)
	if err != nil {
		t.Fatalf("NewAdminUsers returned error: %v", err)
	}
	return admin, store, base
}

func issueFields(t *testing.T, err error) []string {
	t.Helper()
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want a *ValidationError", err)
	}
	var fields []string
	for _, issue := range validation.Issues {
		fields = append(fields, issue.Field)
	}
	return fields
}

func hasField(fields []string, want string) bool {
	for _, field := range fields {
		if field == want {
			return true
		}
	}
	return false
}

func validStaff() CreateStaffInput {
	return CreateStaffInput{
		Email: "Kasun.Courier@SwiftDrop.lk", Password: "TempPass2026", FullName: "Kasun Silva", Phone: "+94712223344",
		Role:    "COURIER",
		Courier: &CourierProfileInput{VehicleType: "BIKE", VehicleNumber: "BCD-4521", HomeZoneID: zoneIDStr},
	}
}

func TestNewAdminUsers(t *testing.T) {
	base := newFixture(t)
	if _, err := NewAdminUsers(nil, newFakeAdminStore()); err == nil {
		t.Fatal("accepted a nil service")
	}
	if _, err := NewAdminUsers(base.service, nil); err == nil {
		t.Fatal("accepted a nil store")
	}
}

func TestAdminUsers_ListUsers_defaultsAndFilters(t *testing.T) {
	admin, store, _ := newAdminFixture(t)
	store.total = 57
	active := true

	page, err := admin.ListUsers(context.Background(), ListUsersInput{Role: "COURIER", IsActive: &active, Search: "  nimal ", Sort: "fullName:desc", Page: 3, PageSize: 10})
	if err != nil {
		t.Fatalf("ListUsers returned error: %v", err)
	}
	filter := store.listFilter
	if filter.Role != auth.RoleCourier || filter.IsActive == nil || !*filter.IsActive || filter.Search != "nimal" ||
		filter.SortField != "fullName" || !filter.SortDesc || filter.Limit != 10 || filter.Offset != 20 {
		t.Fatalf("filter = %+v", filter)
	}
	if page.Page != 3 || page.PageSize != 10 || page.TotalItems != 57 {
		t.Fatalf("page = %+v", page)
	}

	if _, err := admin.ListUsers(context.Background(), ListUsersInput{}); err != nil {
		t.Fatalf("ListUsers with no input returned error: %v", err)
	}
	if f := store.listFilter; f.Limit != 20 || f.Offset != 0 || f.SortField != "createdAt" || !f.SortDesc || f.Role != "" || f.IsActive != nil {
		t.Fatalf("default filter = %+v, want page 1 of 20 newest first", f)
	}
}

func TestAdminUsers_ListUsers_validation(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	tests := []struct {
		name  string
		input ListUsersInput
		field string
	}{
		{name: "unknown role", input: ListUsersInput{Role: "WIZARD"}, field: "role"},
		{name: "lower-case role", input: ListUsersInput{Role: "courier"}, field: "role"},
		{name: "page below 1", input: ListUsersInput{Page: -1}, field: "page"},
		{name: "page size too big", input: ListUsersInput{PageSize: 101}, field: "pageSize"},
		{name: "page size negative", input: ListUsersInput{PageSize: -5}, field: "pageSize"},
		{name: "unknown sort field", input: ListUsersInput{Sort: "password:asc"}, field: "sort"},
		{name: "SQL in sort", input: ListUsersInput{Sort: "createdAt; DROP TABLE users"}, field: "sort"},
		{name: "bad sort direction", input: ListUsersInput{Sort: "email:sideways"}, field: "sort"},
		{name: "search too long", input: ListUsersInput{Search: strings.Repeat("a", maxSearchLength+1)}, field: "q"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := admin.ListUsers(context.Background(), test.input)
			if fields := issueFields(t, err); !hasField(fields, test.field) {
				t.Fatalf("invalid fields = %v, want %s", fields, test.field)
			}
		})
	}
	if store.listFilter.Limit != 0 {
		t.Fatal("the store was queried with an invalid filter")
	}
}

func TestAdminUsers_ListUsers_acceptsMaxPageSizeAndBareSort(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	if _, err := admin.ListUsers(context.Background(), ListUsersInput{PageSize: 100, Sort: "email"}); err != nil {
		t.Fatalf("ListUsers returned error: %v", err)
	}
	if store.listFilter.Limit != 100 || store.listFilter.SortField != "email" || store.listFilter.SortDesc {
		t.Fatalf("filter = %+v", store.listFilter)
	}
}

func TestAdminUsers_ListUsers_storeFailure(t *testing.T) {
	admin, store, _ := newAdminFixture(t)
	store.err = errStore

	if _, err := admin.ListUsers(context.Background(), ListUsersInput{}); !errors.Is(err, errStore) {
		t.Fatalf("error = %v, want the store error", err)
	}
}

func TestAdminUsers_GetUser(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	user, err := admin.GetUser(context.Background(), targetID)
	if err != nil || user.ID != targetID {
		t.Fatalf("GetUser() = %+v, %v", user, err)
	}
	for _, id := range []string{"3f1c7a52-0000-4000-8000-0000000000ff", "nope", ""} {
		if _, err := admin.GetUser(context.Background(), id); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("GetUser(%q) error = %v, want ErrUserNotFound", id, err)
		}
	}
	store.findErr = errStore
	if _, err := admin.GetUser(context.Background(), targetID); !errors.Is(err, errStore) || errors.Is(err, ErrUserNotFound) {
		t.Fatalf("store failure error = %v", err)
	}
}

func TestAdminUsers_CreateStaffUser_courier(t *testing.T) {
	admin, store, base := newAdminFixture(t)

	created, err := admin.CreateStaffUser(context.Background(), adminID, validStaff())
	if err != nil {
		t.Fatalf("CreateStaffUser returned error: %v", err)
	}

	got := store.created
	if got.Email != "kasun.courier@swiftdrop.lk" || got.FullName != "Kasun Silva" || got.Phone != "+94712223344" || got.Role != auth.RoleCourier {
		t.Fatalf("stored params = %+v", got)
	}
	if got.Courier == nil || got.Courier.VehicleType != "BIKE" || got.Courier.VehicleNumber != "BCD-4521" || got.Courier.HomeZoneID != zoneIDStr {
		t.Fatalf("courier profile = %+v", got.Courier)
	}
	if got.PasswordHash == "TempPass2026" || !base.hasher.Matches(got.PasswordHash, "TempPass2026") {
		t.Fatal("the password was not stored as a hash")
	}
	if store.createActor != adminID {
		t.Fatalf("actor = %q, want the calling admin", store.createActor)
	}
	if created.Role != auth.RoleCourier {
		t.Fatalf("created = %+v", created)
	}
}

func TestAdminUsers_CreateStaffUser_dispatcherAndAdminNeedNoProfile(t *testing.T) {
	for _, role := range []string{"DISPATCHER", "ADMIN"} {
		admin, store, _ := newAdminFixture(t)
		input := validStaff()
		input.Role, input.Courier = role, nil

		if _, err := admin.CreateStaffUser(context.Background(), adminID, input); err != nil {
			t.Fatalf("%s: CreateStaffUser returned error: %v", role, err)
		}
		if store.created.Courier != nil || string(store.created.Role) != role {
			t.Fatalf("%s: params = %+v", role, store.created)
		}
	}
}

func TestAdminUsers_CreateStaffUser_validation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*CreateStaffInput)
		field  string
	}{
		{name: "bad email", change: func(i *CreateStaffInput) { i.Email = "nope" }, field: "email"},
		{name: "weak password", change: func(i *CreateStaffInput) { i.Password = "short" }, field: "password"},
		{name: "short name", change: func(i *CreateStaffInput) { i.FullName = "K" }, field: "fullName"},
		{name: "bad phone", change: func(i *CreateStaffInput) { i.Phone = "0712223344" }, field: "phone"},
		{name: "customer role", change: func(i *CreateStaffInput) { i.Role = "CUSTOMER" }, field: "role"},
		{name: "merchant role", change: func(i *CreateStaffInput) { i.Role = "MERCHANT" }, field: "role"},
		{name: "unknown role", change: func(i *CreateStaffInput) { i.Role = "ROOT" }, field: "role"},
		{name: "empty role", change: func(i *CreateStaffInput) { i.Role = "" }, field: "role"},
		{name: "courier without profile", change: func(i *CreateStaffInput) { i.Courier = nil }, field: "courierProfile"},
		{name: "profile for a dispatcher", change: func(i *CreateStaffInput) { i.Role = "DISPATCHER" }, field: "courierProfile"},
		{name: "bad zone id", change: func(i *CreateStaffInput) { i.Courier.HomeZoneID = "zone-1" }, field: "courierProfile.homeZoneId"},
		{name: "long vehicle type", change: func(i *CreateStaffInput) { i.Courier.VehicleType = strings.Repeat("a", 31) }, field: "courierProfile.vehicleType"},
		{name: "long vehicle number", change: func(i *CreateStaffInput) { i.Courier.VehicleNumber = strings.Repeat("a", 21) }, field: "courierProfile.vehicleNumber"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			admin, store, _ := newAdminFixture(t)
			input := validStaff()
			test.change(&input)

			_, err := admin.CreateStaffUser(context.Background(), adminID, input)

			if fields := issueFields(t, err); !hasField(fields, test.field) {
				t.Fatalf("invalid fields = %v, want %s", fields, test.field)
			}
			if store.writes != 0 {
				t.Fatal("an invalid request reached the store")
			}
		})
	}
}

func TestAdminUsers_CreateStaffUser_duplicateEmailAndStoreFailure(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	store.err = repository.ErrEmailTaken
	if _, err := admin.CreateStaffUser(context.Background(), adminID, validStaff()); !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("error = %v, want ErrEmailAlreadyExists", err)
	}
	store.err = errStore
	if _, err := admin.CreateStaffUser(context.Background(), adminID, validStaff()); !errors.Is(err, errStore) {
		t.Fatalf("error = %v, want the store error", err)
	}
}

func ptr[T any](value T) *T { return &value }

func TestAdminUsers_UpdateUser_appliesOnlyGivenFields(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	_, err := admin.UpdateUser(context.Background(), adminID, targetID, UpdateUserInput{
		FullName: ptr("  New Name "), Phone: ptr("+94771234567"), Role: ptr("DISPATCHER"),
		Courier: &CourierProfilePatch{VehicleType: ptr(" VAN "), HomeZoneID: ptr(zoneIDStr), IsAvailable: ptr(false)},
	})
	if err != nil {
		t.Fatalf("UpdateUser returned error: %v", err)
	}

	change := store.updatedChange
	if store.updatedID != targetID || store.updateActor != adminID {
		t.Fatalf("updated %q as %q", store.updatedID, store.updateActor)
	}
	if *change.FullName != "New Name" || *change.Phone != "+94771234567" || *change.Role != auth.RoleDispatcher {
		t.Fatalf("change = %+v", change)
	}
	if c := change.Courier; *c.VehicleType != "VAN" || c.VehicleNumber != nil || *c.HomeZoneID != zoneIDStr || *c.IsAvailable {
		t.Fatalf("courier change = %+v", c)
	}

	if _, err := admin.UpdateUser(context.Background(), adminID, targetID, UpdateUserInput{FullName: ptr("Only Name")}); err != nil {
		t.Fatalf("UpdateUser returned error: %v", err)
	}
	if c := store.updatedChange; c.Phone != nil || c.Role != nil || c.Courier != nil {
		t.Fatalf("an unspecified field was changed: %+v", c)
	}
}

func TestAdminUsers_UpdateUser_validation(t *testing.T) {
	tests := []struct {
		name  string
		input UpdateUserInput
		field string
	}{
		{name: "empty body", input: UpdateUserInput{}, field: "body"},
		{name: "short name", input: UpdateUserInput{FullName: ptr(" a ")}, field: "fullName"},
		{name: "bad phone", input: UpdateUserInput{Phone: ptr("123")}, field: "phone"},
		{name: "unknown role", input: UpdateUserInput{Role: ptr("ROOT")}, field: "role"},
		{name: "merchant role cannot be assigned", input: UpdateUserInput{Role: ptr("MERCHANT")}, field: "role"},
		{name: "bad zone", input: UpdateUserInput{Courier: &CourierProfilePatch{HomeZoneID: ptr("x")}}, field: "courierProfile.homeZoneId"},
		{name: "long plate", input: UpdateUserInput{Courier: &CourierProfilePatch{VehicleNumber: ptr(strings.Repeat("a", 21))}}, field: "courierProfile.vehicleNumber"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			admin, store, _ := newAdminFixture(t)

			_, err := admin.UpdateUser(context.Background(), adminID, targetID, test.input)

			if fields := issueFields(t, err); !hasField(fields, test.field) {
				t.Fatalf("invalid fields = %v, want %s", fields, test.field)
			}
			if store.writes != 0 {
				t.Fatal("an invalid request reached the store")
			}
		})
	}
}

func TestAdminUsers_UpdateUser_notFound(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	for _, id := range []string{"not-a-uuid", ""} {
		if _, err := admin.UpdateUser(context.Background(), adminID, id, UpdateUserInput{FullName: ptr("Valid Name")}); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("UpdateUser(%q) error = %v, want ErrUserNotFound", id, err)
		}
	}
	store.err = repository.ErrNotFound
	if _, err := admin.UpdateUser(context.Background(), adminID, targetID, UpdateUserInput{FullName: ptr("Valid Name")}); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("error = %v, want ErrUserNotFound", err)
	}
	// A role change on an unknown user is also a 404, not a validation error.
	unknown := "3f1c7a52-0000-4000-8000-0000000000ff"
	store.err = nil
	if _, err := admin.UpdateUser(context.Background(), adminID, unknown, UpdateUserInput{Role: ptr("ADMIN")}); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("error = %v, want ErrUserNotFound", err)
	}
}

func TestAdminUsers_UpdateUser_ownRole(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	if _, err := admin.UpdateUser(context.Background(), adminID, adminID, UpdateUserInput{Role: ptr("DISPATCHER")}); !errors.Is(err, ErrCannotChangeOwnRole) {
		t.Fatalf("error = %v, want ErrCannotChangeOwnRole", err)
	}
	if store.writes != 0 {
		t.Fatal("the role was changed")
	}
	// Re-sending the role they already have, and editing other fields, is fine.
	if _, err := admin.UpdateUser(context.Background(), adminID, adminID, UpdateUserInput{Role: ptr("ADMIN"), FullName: ptr("Admin Person")}); err != nil {
		t.Fatalf("UpdateUser returned error: %v", err)
	}
}

func TestAdminUsers_UpdateUser_merchantRoleIsLocked(t *testing.T) {
	admin, store, _ := newAdminFixture(t)
	store.users[targetID] = repository.AdminUser{ID: targetID, Role: auth.RoleMerchant, IsActive: true}

	_, err := admin.UpdateUser(context.Background(), adminID, targetID, UpdateUserInput{Role: ptr("COURIER")})

	if fields := issueFields(t, err); !hasField(fields, "role") {
		t.Fatalf("invalid fields = %v, want role", fields)
	}
	if store.writes != 0 {
		t.Fatal("a merchant's role was changed")
	}
}

func TestAdminUsers_UpdateUser_courierProfileRules(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		field string
	}{
		{name: "courier without profile", err: repository.ErrCourierProfileRequired, field: "courierProfile"},
		{name: "profile for non-courier", err: repository.ErrCourierProfileNotAllowed, field: "courierProfile"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			admin, store, _ := newAdminFixture(t)
			store.err = test.err

			_, err := admin.UpdateUser(context.Background(), adminID, targetID, UpdateUserInput{Role: ptr("COURIER")})

			if fields := issueFields(t, err); !hasField(fields, test.field) {
				t.Fatalf("invalid fields = %v, want %s", fields, test.field)
			}
		})
	}

	admin, store, _ := newAdminFixture(t)
	store.err = errStore
	if _, err := admin.UpdateUser(context.Background(), adminID, targetID, UpdateUserInput{FullName: ptr("Valid Name")}); !errors.Is(err, errStore) {
		t.Fatalf("error = %v, want the store error", err)
	}
}

func TestAdminUsers_SetUserActive(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	if err := admin.SetUserActive(context.Background(), adminID, targetID, false); err != nil {
		t.Fatalf("deactivate returned error: %v", err)
	}
	if store.statusID != targetID || store.statusActive == nil || *store.statusActive || store.statusActor != adminID {
		t.Fatalf("store got %q active=%v actor=%q", store.statusID, store.statusActive, store.statusActor)
	}
	if err := admin.SetUserActive(context.Background(), adminID, targetID, true); err != nil || !*store.statusActive {
		t.Fatalf("activate: err = %v active = %v", err, store.statusActive)
	}
}

func TestAdminUsers_SetUserActive_cannotDeactivateSelf(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	if err := admin.SetUserActive(context.Background(), adminID, adminID, false); !errors.Is(err, ErrCannotDeactivateSelf) {
		t.Fatalf("error = %v, want ErrCannotDeactivateSelf", err)
	}
	if store.writes != 0 {
		t.Fatal("the admin was deactivated")
	}
	// Activating yourself is harmless.
	if err := admin.SetUserActive(context.Background(), adminID, adminID, true); err != nil {
		t.Fatalf("self-activation returned error: %v", err)
	}
}

func TestAdminUsers_SetUserActive_notFoundAndFailure(t *testing.T) {
	admin, store, _ := newAdminFixture(t)

	if err := admin.SetUserActive(context.Background(), adminID, "bad-id", false); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("malformed ID error = %v", err)
	}
	store.err = repository.ErrNotFound
	if err := admin.SetUserActive(context.Background(), adminID, targetID, false); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user error = %v", err)
	}
	store.err = errStore
	if err := admin.SetUserActive(context.Background(), adminID, targetID, false); !errors.Is(err, errStore) {
		t.Fatalf("store failure error = %v", err)
	}
}

func TestAdminUsers_ListCouriers(t *testing.T) {
	admin, store, _ := newAdminFixture(t)
	store.total = 41
	available := true

	page, err := admin.ListCouriers(context.Background(), ListCouriersInput{IsAvailable: &available, ZoneID: " " + zoneIDStr + " ", Page: 2, PageSize: 20})
	if err != nil {
		t.Fatalf("ListCouriers returned error: %v", err)
	}
	f := store.courierFilter
	if f.IsAvailable == nil || !*f.IsAvailable || f.ZoneID != zoneIDStr || f.Limit != 20 || f.Offset != 20 {
		t.Fatalf("filter = %+v", f)
	}
	if page.Page != 2 || page.TotalItems != 41 {
		t.Fatalf("page = %+v", page)
	}

	if _, err := admin.ListCouriers(context.Background(), ListCouriersInput{}); err != nil {
		t.Fatalf("defaults returned error: %v", err)
	}
	if f := store.courierFilter; f.IsAvailable != nil || f.ZoneID != "" || f.Limit != 20 || f.Offset != 0 {
		t.Fatalf("default filter = %+v", f)
	}
}

func TestAdminUsers_ListCouriers_validation(t *testing.T) {
	admin, _, _ := newAdminFixture(t)

	_, err := admin.ListCouriers(context.Background(), ListCouriersInput{ZoneID: "' OR 1=1", PageSize: 500})
	fields := issueFields(t, err)
	if !hasField(fields, "zoneId") || !hasField(fields, "pageSize") {
		t.Fatalf("invalid fields = %v, want zoneId and pageSize", fields)
	}
}

func TestAdminUsers_CurrentAccess(t *testing.T) {
	admin, store, _ := newAdminFixture(t)
	store.users[targetID] = repository.AdminUser{ID: targetID, Role: auth.RoleDispatcher, IsActive: false}

	got, err := admin.CurrentAccess(context.Background(), adminID)
	if err != nil || got != (CurrentAccess{Role: auth.RoleAdmin, Active: true}) {
		t.Fatalf("CurrentAccess(admin) = %+v, %v", got, err)
	}
	got, err = admin.CurrentAccess(context.Background(), targetID)
	if err != nil || got != (CurrentAccess{Role: auth.RoleDispatcher, Active: false}) {
		t.Fatalf("CurrentAccess(deactivated dispatcher) = %+v, %v", got, err)
	}

	for _, id := range []string{"3f1c7a52-0000-4000-8000-0000000000ff", "not-a-uuid", ""} {
		if _, err := admin.CurrentAccess(context.Background(), id); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("CurrentAccess(%q) error = %v, want ErrUserNotFound", id, err)
		}
	}

	store.findErr = errStore
	if _, err := admin.CurrentAccess(context.Background(), adminID); !errors.Is(err, errStore) || errors.Is(err, ErrUserNotFound) {
		t.Fatalf("store failure error = %v, want the store error and not ErrUserNotFound", err)
	}
}
