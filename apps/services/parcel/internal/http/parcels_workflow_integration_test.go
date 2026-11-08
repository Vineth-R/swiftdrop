//go:build integration && postgres

package servicehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/parcels"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

const parcelWorkflowSecret = "parcel-workflow-integration-secret-32-bytes"
const parcelWorkflowInternalToken = "parcel-workflow-internal-test-token"

type httpWorkflowQuotes struct{ total atomic.Int64 }

func (q *httpWorkflowQuotes) Quote(context.Context, parcels.CreateInput) (parcels.Fee, error) {
	total := parcels.Decimal(q.total.Load())
	return parcels.Fee{Currency: "LKR", BaseFee: total - 10000, WeightCharge: 10000, Total: total}, nil
}

func httpWorkflowToken(t *testing.T, customer string, role auth.Role) string {
	t.Helper()
	manager, err := auth.NewTokenManager(parcelWorkflowSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(customer, role)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func httpWorkflowRequest(t *testing.T, router *gin.Engine, method, path, token, internalToken, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-Id", "m5-http-workflow")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if internalToken != "" {
		request.Header.Set("X-Internal-Token", internalToken)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestParcelHTTPWorkflowAgainstPostgres(t *testing.T) {
	database := parceltest.New(t)
	quotes := &httpWorkflowQuotes{}
	quotes.total.Store(55000)
	service := parcels.NewService(parcels.NewStore(database.Pool), quotes)
	gin.SetMode(gin.TestMode)
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Parcels: service, AccessSecret: parcelWorkflowSecret, InternalToken: parcelWorkflowInternalToken})
	if err != nil {
		t.Fatal(err)
	}
	customer := parceltest.UUID(t)
	token := httpWorkflowToken(t, customer, auth.RoleCustomer)
	otherToken := httpWorkflowToken(t, parceltest.UUID(t), auth.RoleCustomer)
	adminToken := httpWorkflowToken(t, parceltest.UUID(t), auth.RoleAdmin)
	dispatcherToken := httpWorkflowToken(t, parceltest.UUID(t), auth.RoleDispatcher)
	input := parcels.CreateInput{
		Sender:    parcels.Contact{Name: "Nimal Perera", Phone: "+94771234567", Address: "12 Temple Road", City: "Colombo", ZoneID: parceltest.UUID(t)},
		Recipient: parcels.Contact{Name: "Amali Fernando", Phone: "+94712345678", Address: "45 Lake Drive", City: "Kandy", ZoneID: parceltest.UUID(t)},
		WeightKG:  200, SizeCategory: "SMALL",
	}
	expected := parcels.Decimal(55000)
	input.ExpectedFee = &expected
	key := parceltest.UUID(t)
	create := httpWorkflowRequest(t, router, http.MethodPost, "/parcels", token, "", key, input)
	if create.Code != http.StatusCreated {
		t.Fatalf("create HTTP %d: %s", create.Code, create.Body.String())
	}
	var envelope struct {
		Data parcels.Detail `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	created := envelope.Data
	if envelope.Meta.RequestID != "m5-http-workflow" || created.ID == "" || created.Fee.Total != expected {
		t.Fatalf("creation envelope lost contract fields: %s", create.Body.String())
	}
	if create.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("customer creation response can be cached")
	}
	for _, suffix := range []string{"", "/history", "/receipt"} {
		path := "/parcels/" + created.ID + suffix
		for _, caller := range []struct {
			name, token string
			want        int
		}{{"owner", token, 200}, {"other customer", otherToken, 404}, {"no auth", "", 401}, {"admin", adminToken, 200}} {
			t.Run(caller.name+suffix, func(t *testing.T) {
				response := httpWorkflowRequest(t, router, http.MethodGet, path, caller.token, "", "", nil)
				if response.Code != caller.want {
					t.Fatalf("HTTP %d, want %d: %s", response.Code, caller.want, response.Body.String())
				}
				if caller.want != 200 && (strings.Contains(response.Body.String(), input.Recipient.Name) || strings.Contains(response.Body.String(), input.Recipient.Phone)) {
					t.Fatal("unauthorized response leaked parcel details")
				}
			})
		}
	}
	for _, path := range []string{"/parcels/" + created.ID, "/parcels/" + created.ID + "/history"} {
		if response := httpWorkflowRequest(t, router, http.MethodGet, path, dispatcherToken, "", "", nil); response.Code != 200 {
			t.Fatalf("dispatcher documented read access rejected: %d %s", response.Code, response.Body.String())
		}
	}
	if response := httpWorkflowRequest(t, router, http.MethodGet, "/parcels/"+created.ID+"/receipt", dispatcherToken, "", "", nil); response.Code != 403 {
		t.Fatalf("dispatcher receipt access = %d, want 403", response.Code)
	}
	for _, tc := range []struct {
		path string
		want int
	}{{"/parcels?page=1&pageSize=1&sort=createdAt:asc&status=CREATED", 200}, {"/parcels?page=0", 422}, {"/parcels?pageSize=101", 422}, {"/parcels?status=INVALID", 422}, {"/parcels?sort=createdAt:drop", 422}, {"/parcels?from=2026-01-02&to=2026-01-01", 422}} {
		response := httpWorkflowRequest(t, router, http.MethodGet, tc.path, token, "", "", nil)
		if response.Code != tc.want {
			t.Fatalf("list %s HTTP %d, want %d: %s", tc.path, response.Code, tc.want, response.Body.String())
		}
		if tc.want == 200 {
			var body struct {
				Data []parcels.ListItem `json:"data"`
				Meta struct {
					Pagination struct{ Page, PageSize, TotalItems, TotalPages int } `json:"pagination"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Data) != 1 || body.Meta.Pagination.Page != 1 || body.Meta.Pagination.PageSize != 1 || body.Meta.Pagination.TotalItems != 1 || body.Meta.Pagination.TotalPages != 1 {
				t.Fatalf("list pagination envelope: %s; error %v", response.Body.String(), err)
			}
		}
	}
	quotes.total.Store(60000)
	changed := httpWorkflowRequest(t, router, http.MethodPost, "/parcels", token, "", parceltest.UUID(t), input)
	var changedEnvelope struct {
		Error struct {
			Code       string          `json:"code"`
			CurrentFee parcels.Decimal `json:"currentFee"`
		} `json:"error"`
	}
	if err := json.Unmarshal(changed.Body.Bytes(), &changedEnvelope); err != nil || changed.Code != 409 || changedEnvelope.Error.Code != "PRICE_CHANGED" || changedEnvelope.Error.CurrentFee != 60000 {
		t.Fatalf("price change response: HTTP %d %s; error %v", changed.Code, changed.Body.String(), err)
	}
	replay := httpWorkflowRequest(t, router, http.MethodPost, "/parcels", token, "", key, input)
	if replay.Code != 201 || !strings.Contains(replay.Body.String(), created.ID) {
		t.Fatalf("creation replay after price change: %d %s", replay.Code, replay.Body.String())
	}
	receipt := httpWorkflowRequest(t, router, http.MethodGet, "/parcels/"+created.ID+"/receipt", token, "", "", nil)
	if receipt.Code != 200 || receipt.Header().Get("Content-Type") != "application/pdf" || receipt.Header().Get("Content-Disposition") != `attachment; filename="receipt_`+created.TrackingNumber+`.pdf"` || !strings.Contains(receipt.Body.String(), "Total fee: 550.00") {
		t.Fatalf("receipt headers or stored price are wrong: %d %v", receipt.Code, receipt.Header())
	}
	for _, route := range []string{"/parcels/" + parceltest.UUID(t), "/parcels/" + parceltest.UUID(t) + "/history", "/parcels/" + parceltest.UUID(t) + "/receipt"} {
		if response := httpWorkflowRequest(t, router, http.MethodGet, route, token, "", "", nil); response.Code != 404 {
			t.Fatalf("missing parcel %s HTTP %d", route, response.Code)
		}
	}
	merchant := parcels.MerchantInput{MerchantID: parceltest.UUID(t), CreatedByUserID: parceltest.UUID(t), ExternalOrderID: "HTTP-SHOP-100", IdempotencyKey: "HTTP-SHOP-100", Pickup: input.Sender, Recipient: input.Recipient, WeightKG: input.WeightKG, SizeCategory: input.SizeCategory}
	for _, internalToken := range []string{"", "incorrect"} {
		response := httpWorkflowRequest(t, router, http.MethodPost, "/internal/parcels", "", internalToken, "", merchant)
		if response.Code != 401 {
			t.Fatalf("internal token %q accepted: %d", internalToken, response.Code)
		}
		response = httpWorkflowRequest(t, router, http.MethodGet, "/internal/parcels/"+created.ID+"/summary", "", internalToken, "", nil)
		if response.Code != 401 {
			t.Fatalf("internal summary token %q accepted: %d", internalToken, response.Code)
		}
	}
	merchantCreate := httpWorkflowRequest(t, router, http.MethodPost, "/internal/parcels", "", parcelWorkflowInternalToken, "", merchant)
	var merchantEnvelope struct {
		Data struct {
			ParcelID       string `json:"parcelId"`
			TrackingNumber string `json:"trackingNumber"`
		} `json:"data"`
	}
	if err := json.Unmarshal(merchantCreate.Body.Bytes(), &merchantEnvelope); err != nil || merchantCreate.Code != 201 || merchantEnvelope.Data.ParcelID == "" || !strings.HasPrefix(merchantEnvelope.Data.TrackingNumber, "SDN") {
		t.Fatalf("merchant creation HTTP %d: %s; error %v", merchantCreate.Code, merchantCreate.Body.String(), err)
	}
	summary := httpWorkflowRequest(t, router, http.MethodGet, "/internal/parcels/"+merchantEnvelope.Data.ParcelID+"/summary", "", parcelWorkflowInternalToken, "", nil)
	if summary.Code != 200 || !strings.Contains(summary.Body.String(), merchant.MerchantID) || strings.Contains(summary.Body.String(), merchant.Recipient.Phone) || strings.Contains(summary.Body.String(), merchant.Recipient.Address) {
		t.Fatalf("merchant summary failed or exposed unnecessary data: %d %s", summary.Code, summary.Body.String())
	}
	missingSummary := httpWorkflowRequest(t, router, http.MethodGet, "/internal/parcels/"+parceltest.UUID(t)+"/summary", "", parcelWorkflowInternalToken, "", nil)
	if missingSummary.Code != 404 {
		t.Fatalf("missing summary HTTP %d", missingSummary.Code)
	}
}
