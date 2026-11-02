package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const (
	adminUserID  = "3f1c7a52-0000-4000-8000-0000000000a1"
	targetUserID = "3f1c7a52-0000-4000-8000-0000000000b2"
)

type fakeAdminService struct {
	page         service.UserPage
	couriers     service.CourierPage
	user         service.AdminUser
	err          error
	calls        int
	listInput    service.ListUsersInput
	createInput  service.CreateStaffInput
	updateInput  service.UpdateUserInput
	courierInput service.ListCouriersInput
	actor        string
	gotUserID    string
	gotActive    *bool

	// access is what CurrentAccess reports; nil means an active ADMIN.
	access       *service.CurrentAccess
	accessErr    error
	accessCalled string
}

func (f *fakeAdminService) ListUsers(_ context.Context, in service.ListUsersInput) (service.UserPage, error) {
	f.calls++
	f.listInput = in
	return f.page, f.err
}

func (f *fakeAdminService) GetUser(_ context.Context, id string) (service.AdminUser, error) {
	f.calls++
	f.gotUserID = id
	return f.user, f.err
}

func (f *fakeAdminService) CreateStaffUser(_ context.Context, actor string, in service.CreateStaffInput) (service.AdminUser, error) {
	f.calls++
	f.actor, f.createInput = actor, in
	return f.user, f.err
}

func (f *fakeAdminService) UpdateUser(_ context.Context, actor, id string, in service.UpdateUserInput) (service.AdminUser, error) {
	f.calls++
	f.actor, f.gotUserID, f.updateInput = actor, id, in
	return f.user, f.err
}

func (f *fakeAdminService) SetUserActive(_ context.Context, actor, id string, active bool) error {
	f.calls++
	f.actor, f.gotUserID, f.gotActive = actor, id, &active
	return f.err
}

func (f *fakeAdminService) CurrentAccess(_ context.Context, userID string) (service.CurrentAccess, error) {
	f.accessCalled = userID
	if f.accessErr != nil {
		return service.CurrentAccess{}, f.accessErr
	}
	if f.access != nil {
		return *f.access, nil
	}
	return service.CurrentAccess{Role: auth.RoleAdmin, Active: true}, nil
}

func (f *fakeAdminService) ListCouriers(_ context.Context, in service.ListCouriersInput) (service.CourierPage, error) {
	f.calls++
	f.courierInput = in
	return f.couriers, f.err
}

type adminHarness struct {
	service *fakeAdminService
	handler http.Handler
	tokens  *auth.TokenManager
}

func newAdminHarness(t *testing.T) *adminHarness {
	t.Helper()
	tokens, err := auth.NewTokenManager(handlerTestSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}
	phone := "+94771234567"
	zone := "7a1e0000-0000-4000-8000-000000000001"
	vehicle := "BIKE"
	login := time.Date(2026, time.November, 3, 8, 0, 0, 0, time.UTC)
	created := time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC)
	fake := &fakeAdminService{
		user: service.AdminUser{
			ID: targetUserID, Email: "kasun@swiftdrop.lk", FullName: "Kasun Silva", Phone: &phone,
			Role: auth.RoleCourier, IsActive: true, CreatedAt: created, UpdatedAt: created.Add(time.Hour), LastLoginAt: &login,
			Courier: &repository.CourierProfile{VehicleType: &vehicle, HomeZoneID: &zone, IsAvailable: true},
		},
	}
	handler := New(slog.New(slog.DiscardHandler), WithAdminUsers(AdminRoutes{
		Service:  fake,
		Verifier: tokens,
		Cookies:  CookieConfig{AccessName: "sd_access", RefreshName: "sd_refresh"},
	}))
	return &adminHarness{service: fake, handler: handler, tokens: tokens}
}

func (h *adminHarness) token(t *testing.T, userID string, role auth.Role) string {
	t.Helper()
	token, _, err := h.tokens.Issue(userID, role)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return token
}

// do sends a request as the given role (empty role means no credentials).
func (h *adminHarness) do(t *testing.T, role auth.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		request.Header.Set("Authorization", "Bearer "+h.token(t, adminUserID, role))
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

var adminEndpoints = []struct{ method, path, body string }{
	{http.MethodGet, "/admin/users", ""},
	{http.MethodPost, "/admin/users", `{}`},
	{http.MethodGet, "/admin/users/" + targetUserID, ""},
	{http.MethodPatch, "/admin/users/" + targetUserID, `{"fullName":"New Name"}`},
	{http.MethodPatch, "/admin/users/" + targetUserID + "/status", `{"isActive":false}`},
}

func TestAdminEndpoints_requireAuthentication(t *testing.T) {
	h := newAdminHarness(t)

	for _, endpoint := range append(adminEndpoints, struct{ method, path, body string }{http.MethodGet, "/couriers", ""}) {
		response := h.do(t, "", endpoint.method, endpoint.path, endpoint.body)
		if response.Code != http.StatusUnauthorized || errorCode(t, response) != "UNAUTHENTICATED" {
			t.Fatalf("%s %s status = %d body = %s, want 401 UNAUTHENTICATED", endpoint.method, endpoint.path, response.Code, response.Body)
		}
	}
	if h.service.calls != 0 {
		t.Fatal("an unauthenticated request reached the service")
	}
}

func TestAdminEndpoints_onlyAdminMayUseThem(t *testing.T) {
	h := newAdminHarness(t)

	for _, role := range []auth.Role{auth.RoleCustomer, auth.RoleMerchant, auth.RoleCourier, auth.RoleDispatcher} {
		for _, endpoint := range adminEndpoints {
			response := h.do(t, role, endpoint.method, endpoint.path, endpoint.body)
			if response.Code != http.StatusForbidden || errorCode(t, response) != "FORBIDDEN" {
				t.Fatalf("%s %s %s status = %d, want 403 FORBIDDEN", role, endpoint.method, endpoint.path, response.Code)
			}
		}
	}
	if h.service.calls != 0 {
		t.Fatal("a non-admin request reached the service")
	}

	for _, endpoint := range adminEndpoints {
		if response := h.do(t, auth.RoleAdmin, endpoint.method, endpoint.path, endpoint.body); response.Code == http.StatusForbidden || response.Code == http.StatusUnauthorized {
			t.Fatalf("ADMIN %s %s was refused: %d", endpoint.method, endpoint.path, response.Code)
		}
	}
}

func TestAdminEndpoints_acceptTheSessionCookie(t *testing.T) {
	h := newAdminHarness(t)
	request := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	request.AddCookie(&http.Cookie{Name: "sd_access", Value: h.token(t, adminUserID, auth.RoleAdmin)})
	recorder := httptest.NewRecorder()

	h.handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with the access cookie", recorder.Code)
	}
}

func TestCouriersEndpoint_dispatcherAndAdminOnly(t *testing.T) {
	h := newAdminHarness(t)

	for _, role := range []auth.Role{auth.RoleDispatcher, auth.RoleAdmin} {
		if response := h.do(t, role, http.MethodGet, "/couriers", ""); response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", role, response.Code)
		}
	}
	for _, role := range []auth.Role{auth.RoleCustomer, auth.RoleMerchant, auth.RoleCourier} {
		if response := h.do(t, role, http.MethodGet, "/couriers", ""); response.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d, want 403", role, response.Code)
		}
	}
}

func TestCouriersEndpoint_responseAndQuery(t *testing.T) {
	h := newAdminHarness(t)
	phone := "+94712223344"
	vehicle := "BIKE"
	zone := "7a1e0000-0000-4000-8000-000000000001"
	h.service.couriers = service.CourierPage{
		Couriers:   []service.CourierListing{{ID: "c1d2", FullName: "Kasun Silva", Phone: &phone, VehicleType: &vehicle, HomeZoneID: &zone, IsAvailable: true}},
		Page:       1,
		PageSize:   20,
		TotalItems: 1,
	}

	response := h.do(t, auth.RoleDispatcher, http.MethodGet, "/couriers?isAvailable=true&zoneId="+zone+"&page=1&pageSize=20", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", response.Code, response.Body)
	}
	var body struct {
		Data []json.RawMessage `json:"data"`
		Meta struct {
			Pagination map[string]int `json:"pagination"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Data) != 1 {
		t.Fatalf("body = %s (%v)", response.Body, err)
	}
	want := `{"id":"c1d2","fullName":"Kasun Silva","phone":"+94712223344","vehicleType":"BIKE","homeZoneId":"` + zone + `","isAvailable":true}`
	if string(body.Data[0]) != want {
		t.Fatalf("item = %s, want %s", body.Data[0], want)
	}
	if p := body.Meta.Pagination; p["page"] != 1 || p["pageSize"] != 20 || p["totalItems"] != 1 || p["totalPages"] != 1 {
		t.Fatalf("pagination = %v", p)
	}
	in := h.service.courierInput
	if in.IsAvailable == nil || !*in.IsAvailable || in.ZoneID != zone || in.Page != 1 || in.PageSize != 20 {
		t.Fatalf("service input = %+v", in)
	}
}

func TestCouriersEndpoint_emptyListIsAnArrayNotNull(t *testing.T) {
	h := newAdminHarness(t)

	response := h.do(t, auth.RoleDispatcher, http.MethodGet, "/couriers", "")

	if !strings.Contains(response.Body.String(), `"data":[]`) {
		t.Fatalf("body = %s, want an empty array", response.Body)
	}
}

func TestListUsers_responseShapeAndQuery(t *testing.T) {
	h := newAdminHarness(t)
	phone := "+94771234567"
	h.service.page = service.UserPage{
		Users: []service.AdminUser{{
			ID: "3f1c", Email: "nimal@example.com", FullName: "Nimal Perera", Phone: &phone,
			Role: auth.RoleCustomer, IsActive: true, CreatedAt: time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC),
			Courier: &repository.CourierProfile{},
		}},
		Page: 1, PageSize: 20, TotalItems: 57,
	}

	response := h.do(t, auth.RoleAdmin, http.MethodGet, "/admin/users?role=CUSTOMER&isActive=true&q=nimal&sort=createdAt:desc&page=1&pageSize=20", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", response.Code, response.Body)
	}
	var body struct {
		Data []json.RawMessage `json:"data"`
		Meta struct {
			Pagination map[string]int `json:"pagination"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	want := `{"id":"3f1c","email":"nimal@example.com","fullName":"Nimal Perera","phone":"+94771234567","role":"CUSTOMER","isActive":true,"createdAt":"2026-11-02T09:30:00Z"}`
	if len(body.Data) != 1 || string(body.Data[0]) != want {
		t.Fatalf("data = %s, want [%s]", response.Body, want)
	}
	if body.Meta.Pagination["totalItems"] != 57 || body.Meta.Pagination["totalPages"] != 3 {
		t.Fatalf("pagination = %v", body.Meta.Pagination)
	}
	in := h.service.listInput
	if in.Role != "CUSTOMER" || in.IsActive == nil || !*in.IsActive || in.Search != "nimal" || in.Sort != "createdAt:desc" || in.Page != 1 || in.PageSize != 20 {
		t.Fatalf("service input = %+v", in)
	}
}

func TestListUsers_malformedQueryIsOne422WithEveryProblem(t *testing.T) {
	h := newAdminHarness(t)

	response := h.do(t, auth.RoleAdmin, http.MethodGet, "/admin/users?isActive=maybe&page=abc&pageSize=1.5", "")

	if response.Code != http.StatusUnprocessableEntity || errorCode(t, response) != "VALIDATION_ERROR" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
	for _, field := range []string{`"isActive"`, `"page"`, `"pageSize"`} {
		if !strings.Contains(response.Body.String(), field) {
			t.Fatalf("details miss %s: %s", field, response.Body)
		}
	}
	if h.service.calls != 0 {
		t.Fatal("the service was called with a malformed query")
	}
}

func TestListUsers_serviceValidationErrorBecomes422(t *testing.T) {
	h := newAdminHarness(t)
	h.service.err = &service.ValidationError{Issues: []service.Issue{{Field: "role", Issue: "must be one of ..."}}}

	response := h.do(t, auth.RoleAdmin, http.MethodGet, "/admin/users?role=WIZARD", "")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"field":"role"`) {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
}

func TestAdminUsers_collectionWorksWithAndWithoutTrailingSlash(t *testing.T) {
	h := newAdminHarness(t)

	for _, path := range []string{"/admin/users", "/admin/users/"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			response := h.do(t, auth.RoleAdmin, method, path, `{}`)
			if response.Code == http.StatusMovedPermanently || response.Code == http.StatusTemporaryRedirect || response.Code == http.StatusNotFound {
				t.Fatalf("%s %s status = %d; behind the gateway a redirect would drop /api/v1", method, path, response.Code)
			}
		}
	}
	for _, path := range []string{"/couriers", "/couriers/"} {
		if response := h.do(t, auth.RoleDispatcher, http.MethodGet, path, ""); response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, response.Code)
		}
	}
}

func TestCreateUser_success(t *testing.T) {
	h := newAdminHarness(t)
	body := `{"email":"kasun@swiftdrop.lk","password":"TempPass#2026","fullName":"Kasun Silva","phone":"+94712223344","role":"COURIER",
	"courierProfile":{"vehicleType":"BIKE","vehicleNumber":"BCD-4521","homeZoneId":"7a1e0000-0000-4000-8000-000000000001"}}`

	response := h.do(t, auth.RoleAdmin, http.MethodPost, "/admin/users", body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d (%s)", response.Code, response.Body)
	}
	in := h.service.createInput
	if h.service.actor != adminUserID || in.Email != "kasun@swiftdrop.lk" || in.Role != "COURIER" || in.Courier == nil ||
		in.Courier.VehicleNumber != "BCD-4521" || in.Courier.HomeZoneID != "7a1e0000-0000-4000-8000-000000000001" {
		t.Fatalf("service input = %+v actor = %q", in, h.service.actor)
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &payload)
	for _, field := range []string{"id", "email", "fullName", "phone", "role", "isActive", "createdAt", "courierProfile"} {
		if _, ok := payload.Data[field]; !ok {
			t.Fatalf("response misses %q: %s", field, response.Body)
		}
	}
	if strings.Contains(response.Body.String(), "TempPass") || strings.Contains(response.Body.String(), "password") {
		t.Fatal("the response mentions the password")
	}
}

func TestCreateUser_errors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "duplicate email", err: service.ErrEmailAlreadyExists, status: http.StatusConflict, code: "EMAIL_ALREADY_EXISTS"},
		{name: "validation", err: &service.ValidationError{Issues: []service.Issue{{Field: "role", Issue: "bad"}}}, status: http.StatusUnprocessableEntity, code: "VALIDATION_ERROR"},
		{name: "unexpected", err: errors.New("db down"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newAdminHarness(t)
			h.service.err = test.err

			response := h.do(t, auth.RoleAdmin, http.MethodPost, "/admin/users", `{"email":"a@b.lk"}`)

			if response.Code != test.status || errorCode(t, response) != test.code {
				t.Fatalf("status = %d body = %s, want %d %s", response.Code, response.Body, test.status, test.code)
			}
		})
	}
}

func TestCreateUser_badBodies(t *testing.T) {
	h := newAdminHarness(t)

	if response := h.do(t, auth.RoleAdmin, http.MethodPost, "/admin/users", `{not json`); response.Code != http.StatusBadRequest {
		t.Fatalf("malformed JSON status = %d, want 400", response.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader("email=a"))
	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Authorization", "Bearer "+h.token(t, adminUserID, auth.RoleAdmin))
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content type status = %d, want 415", recorder.Code)
	}
}

func TestGetUser_fullView(t *testing.T) {
	h := newAdminHarness(t)

	response := h.do(t, auth.RoleAdmin, http.MethodGet, "/admin/users/"+targetUserID, "")

	if response.Code != http.StatusOK || h.service.gotUserID != targetUserID {
		t.Fatalf("status = %d id = %q", response.Code, h.service.gotUserID)
	}
	var payload struct {
		Data struct {
			LastLoginAt    string `json:"lastLoginAt"`
			UpdatedAt      string `json:"updatedAt"`
			CourierProfile struct {
				VehicleType string `json:"vehicleType"`
				IsAvailable bool   `json:"isAvailable"`
			} `json:"courierProfile"`
		} `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &payload)
	d := payload.Data
	if d.LastLoginAt != "2026-11-03T08:00:00Z" || d.UpdatedAt != "2026-11-02T10:30:00Z" || d.CourierProfile.VehicleType != "BIKE" || !d.CourierProfile.IsAvailable {
		t.Fatalf("data = %s", response.Body)
	}
}

func TestGetUser_neverLoggedInIsNullAndNonCourierHasNoProfile(t *testing.T) {
	h := newAdminHarness(t)
	h.service.user = service.AdminUser{ID: targetUserID, Role: auth.RoleCustomer}

	response := h.do(t, auth.RoleAdmin, http.MethodGet, "/admin/users/"+targetUserID, "")

	if !strings.Contains(response.Body.String(), `"lastLoginAt":null`) || strings.Contains(response.Body.String(), "courierProfile") {
		t.Fatalf("body = %s", response.Body)
	}
}

func TestUserEndpoints_notFoundIs404(t *testing.T) {
	for _, endpoint := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/users/" + targetUserID, ""},
		{http.MethodPatch, "/admin/users/" + targetUserID, `{"fullName":"New Name"}`},
		{http.MethodPatch, "/admin/users/" + targetUserID + "/status", `{"isActive":true}`},
	} {
		h := newAdminHarness(t)
		h.service.err = service.ErrUserNotFound

		response := h.do(t, auth.RoleAdmin, endpoint.method, endpoint.path, endpoint.body)

		if response.Code != http.StatusNotFound || errorCode(t, response) != "NOT_FOUND" {
			t.Fatalf("%s %s status = %d body = %s, want 404 NOT_FOUND", endpoint.method, endpoint.path, response.Code, response.Body)
		}
	}
}

func TestUpdateUser_passesOnlyGivenFields(t *testing.T) {
	h := newAdminHarness(t)

	response := h.do(t, auth.RoleAdmin, http.MethodPatch, "/admin/users/"+targetUserID,
		`{"fullName":"New Name","courierProfile":{"isAvailable":false}}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", response.Code, response.Body)
	}
	in := h.service.updateInput
	if h.service.actor != adminUserID || h.service.gotUserID != targetUserID {
		t.Fatalf("actor %q target %q", h.service.actor, h.service.gotUserID)
	}
	if in.FullName == nil || *in.FullName != "New Name" || in.Phone != nil || in.Role != nil {
		t.Fatalf("input = %+v", in)
	}
	if in.Courier == nil || in.Courier.IsAvailable == nil || *in.Courier.IsAvailable || in.Courier.VehicleType != nil {
		t.Fatalf("courier input = %+v", in.Courier)
	}
}

func TestUpdateUser_conflictsAndValidation(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "own role", err: service.ErrCannotChangeOwnRole, status: http.StatusConflict, code: "CONFLICT"},
		{name: "validation", err: &service.ValidationError{Issues: []service.Issue{{Field: "role", Issue: "bad"}}}, status: http.StatusUnprocessableEntity, code: "VALIDATION_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newAdminHarness(t)
			h.service.err = test.err

			response := h.do(t, auth.RoleAdmin, http.MethodPatch, "/admin/users/"+targetUserID, `{"role":"DISPATCHER"}`)

			if response.Code != test.status || errorCode(t, response) != test.code {
				t.Fatalf("status = %d body = %s", response.Code, response.Body)
			}
		})
	}
}

func TestSetStatus(t *testing.T) {
	h := newAdminHarness(t)

	response := h.do(t, auth.RoleAdmin, http.MethodPatch, "/admin/users/"+targetUserID+"/status", `{"isActive":false}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", response.Code, response.Body)
	}
	var payload struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &payload)
	if want := `{"id":"` + targetUserID + `","isActive":false}`; string(payload.Data) != want {
		t.Fatalf("data = %s, want %s", payload.Data, want)
	}
	if h.service.actor != adminUserID || h.service.gotActive == nil || *h.service.gotActive {
		t.Fatalf("service got actor %q active %v", h.service.actor, h.service.gotActive)
	}
}

func TestSetStatus_isActiveIsRequired(t *testing.T) {
	h := newAdminHarness(t)

	for _, body := range []string{`{}`, `{"isActive":null}`} {
		response := h.do(t, auth.RoleAdmin, http.MethodPatch, "/admin/users/"+targetUserID+"/status", body)
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"field":"isActive"`) {
			t.Fatalf("body %s: status = %d (%s)", body, response.Code, response.Body)
		}
	}
	if h.service.calls != 0 {
		t.Fatal("the service was called without isActive")
	}
}

func TestSetStatus_cannotDeactivateSelf(t *testing.T) {
	h := newAdminHarness(t)
	h.service.err = service.ErrCannotDeactivateSelf

	response := h.do(t, auth.RoleAdmin, http.MethodPatch, "/admin/users/"+adminUserID+"/status", `{"isActive":false}`)

	if response.Code != http.StatusConflict || errorCode(t, response) != "CANNOT_DEACTIVATE_SELF" {
		t.Fatalf("status = %d body = %s, want 409 CANNOT_DEACTIVATE_SELF", response.Code, response.Body)
	}
}

func TestAdminEndpoints_unexpectedFailuresAreGeneric(t *testing.T) {
	h := newAdminHarness(t)
	h.service.err = errors.New("pq: relation identity.users is locked")

	response := h.do(t, auth.RoleAdmin, http.MethodGet, "/admin/users", "")

	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "relation") {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
}

func TestAdminEndpoints_rateLimitedPerUser(t *testing.T) {
	h := newAdminHarness(t)

	for i := 0; i < userRequestsPerMinute; i++ {
		if response := h.do(t, auth.RoleAdmin, http.MethodGet, "/couriers", ""); response.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i+1, response.Code)
		}
	}
	response := h.do(t, auth.RoleAdmin, http.MethodGet, "/couriers", "")
	if response.Code != http.StatusTooManyRequests || errorCode(t, response) != "RATE_LIMITED" {
		t.Fatalf("status = %d, want 429 RATE_LIMITED", response.Code)
	}

	// Another user keeps their own budget.
	other := httptest.NewRequest(http.MethodGet, "/couriers", nil)
	token, _, _ := h.tokens.Issue("3f1c7a52-0000-4000-8000-0000000000c3", auth.RoleDispatcher)
	other.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, other)
	if recorder.Code != http.StatusOK {
		t.Fatalf("a different user was limited: %d", recorder.Code)
	}
}

func TestRouter_withoutAdminHasNoAdminRoutes(t *testing.T) {
	handler := New(slog.New(slog.DiscardHandler))

	for _, path := range []string{"/admin/users", "/couriers"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, recorder.Code)
		}
	}
}

// A signed access token keeps its role until it expires. These tests prove the
// admin and courier endpoints look at the caller's account as it is now.
func TestAdminEndpoints_refuseACallerWhoseAccountChangedAfterTheTokenWasIssued(t *testing.T) {
	tests := []struct {
		name   string
		access *service.CurrentAccess
		err    error
		status int
		code   string
	}{
		{name: "deactivated since", access: &service.CurrentAccess{Role: auth.RoleAdmin, Active: false}, status: http.StatusForbidden, code: "ACCOUNT_DISABLED"},
		{name: "demoted since", access: &service.CurrentAccess{Role: auth.RoleCustomer, Active: true}, status: http.StatusForbidden, code: "FORBIDDEN"},
		{name: "demoted to dispatcher", access: &service.CurrentAccess{Role: auth.RoleDispatcher, Active: true}, status: http.StatusForbidden, code: "FORBIDDEN"},
		{name: "account deleted", err: service.ErrUserNotFound, status: http.StatusUnauthorized, code: "UNAUTHENTICATED"},
		{name: "lookup failed", err: errors.New("db down at 10.0.0.5"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, test := range tests {
		for _, endpoint := range adminEndpoints {
			t.Run(test.name+" "+endpoint.method+" "+endpoint.path, func(t *testing.T) {
				h := newAdminHarness(t)
				h.service.access, h.service.accessErr = test.access, test.err

				response := h.do(t, auth.RoleAdmin, endpoint.method, endpoint.path, endpoint.body)

				if response.Code != test.status || errorCode(t, response) != test.code {
					t.Fatalf("status = %d body = %s, want %d %s", response.Code, response.Body, test.status, test.code)
				}
				if strings.Contains(response.Body.String(), "10.0.0.5") {
					t.Fatal("internal details leaked to the caller")
				}
				if h.service.calls != 0 {
					t.Fatal("the request reached the handler")
				}
				if h.service.accessCalled != adminUserID {
					t.Fatalf("the caller's account was looked up as %q, want the token's subject", h.service.accessCalled)
				}
			})
		}
	}
}

func TestCouriersEndpoint_refusesADeactivatedDispatcherWithAValidToken(t *testing.T) {
	h := newAdminHarness(t)
	h.service.access = &service.CurrentAccess{Role: auth.RoleDispatcher, Active: false}

	response := h.do(t, auth.RoleDispatcher, http.MethodGet, "/couriers", "")

	if response.Code != http.StatusForbidden || errorCode(t, response) != "ACCOUNT_DISABLED" {
		t.Fatalf("status = %d body = %s, want 403 ACCOUNT_DISABLED", response.Code, response.Body)
	}
}

func TestCouriersEndpoint_acceptsAnActiveDispatcherAndAdmin(t *testing.T) {
	for _, role := range []auth.Role{auth.RoleDispatcher, auth.RoleAdmin} {
		h := newAdminHarness(t)
		h.service.access = &service.CurrentAccess{Role: role, Active: true}

		if response := h.do(t, role, http.MethodGet, "/couriers", ""); response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", role, response.Code)
		}
	}
}

func TestAdminEndpoints_unauthenticatedCallersDoNotTriggerAnAccountLookup(t *testing.T) {
	h := newAdminHarness(t)

	h.do(t, "", http.MethodGet, "/admin/users", "")
	h.do(t, auth.RoleCustomer, http.MethodGet, "/admin/users", "")

	if h.service.accessCalled != "" {
		t.Fatal("the database was consulted for a caller the token check already refused")
	}
}
