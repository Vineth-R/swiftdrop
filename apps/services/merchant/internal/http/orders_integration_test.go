//go:build integration

package servicehttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/apikey"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/parcelclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

type staticZones struct{ name string }

func (z staticZones) ZoneName(context.Context, string) (string, error) { return z.name, nil }

type fakeParcel struct {
	mu        sync.Mutex
	byOrder   map[string][]byte
	responses map[string]orders.ParcelResult
	timeouts  map[string]bool
	calls     int
}

func newFakeParcel() *fakeParcel {
	return &fakeParcel{
		byOrder: map[string][]byte{}, responses: map[string]orders.ParcelResult{},
		timeouts: map[string]bool{},
	}
}

func (f *fakeParcel) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/parcels" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var input struct {
			ExternalOrderID string `json:"externalOrderId"`
			Recipient       struct {
				Name string `json:"name"`
			} `json:"recipient"`
			WeightKG     float64 `json:"weightKg"`
			SizeCategory string  `json:"sizeCategory"`
		}
		_ = json.Unmarshal(body, &input)

		f.mu.Lock()
		if input.ExternalOrderID == "SZ-ROUTE-FAIL" {
			f.mu.Unlock()
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"error":{"code":"ROUTE_NOT_SERVICEABLE","message":"no route"}}`)
			return
		}
		if !f.timeouts[input.ExternalOrderID] && input.ExternalOrderID == "SZ-TIMEOUT" {
			f.timeouts[input.ExternalOrderID] = true
			f.mu.Unlock()
			time.Sleep(150 * time.Millisecond)
			return
		}
		f.calls++
		if previous, ok := f.byOrder[input.ExternalOrderID]; ok {
			if string(previous) != string(body) {
				f.mu.Unlock()
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"error":{"code":"IDEMPOTENCY_CONFLICT"}}`)
				return
			}
			result := f.responses[input.ExternalOrderID]
			f.mu.Unlock()
			writeParcelSuccess(w, result)
			return
		}
		result := orders.ParcelResult{
			ParcelID:       fmt.Sprintf("00000000-0000-4000-8000-%012d", f.calls),
			TrackingNumber: fmt.Sprintf("SDN%08d", f.calls),
			Status:         "CREATED",
			Fee:            orders.Fee{Currency: "LKR", BaseFee: 45000, WeightCharge: 10000, Total: 55000},
		}
		f.byOrder[input.ExternalOrderID] = body
		f.responses[input.ExternalOrderID] = result
		f.mu.Unlock()
		writeParcelSuccess(w, result)
	}))
}

func writeParcelSuccess(w http.ResponseWriter, result orders.ParcelResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"parcelId": result.ParcelID, "trackingNumber": result.TrackingNumber,
		"status": result.Status, "fee": result.Fee,
	}})
}

func newOrderHTTPFixture(t *testing.T) (context.Context, *repository.Repository, *pgxpool.Pool, *gin.Engine, *fakeParcel) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping merchant order HTTP tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	adminConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}
	databaseName := "swiftdrop_merchant_order_http_" + testHex(8)
	quoted := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create isolated database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Logf("drop isolated database %s: %v", databaseName, err)
		}
		_ = admin.Close(cleanupCtx)
	})
	migrationConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse migration URL: %v", err)
	}
	migrationConfig.Database = databaseName
	migrationConn, err := pgx.ConnectConfig(ctx, migrationConfig)
	if err != nil {
		t.Fatalf("connect to isolated database: %v", err)
	}
	t.Cleanup(func() { _ = migrationConn.Close(context.Background()) })
	root := filepath.Join("..", "..", "..", "..", "..")
	applySQL(t, ctx, migrationConn, filepath.Join(root, "infra/docker/postgres/init/01-schemas.sql"))
	migrations, err := filepath.Glob(filepath.Join(root, "migrations/merchant/*.up.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find merchant migrations: %v", err)
	}
	sort.Strings(migrations)
	for _, migration := range migrations {
		applySQL(t, ctx, migrationConn, migration)
	}
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	poolConfig.ConnConfig.Database = databaseName
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	repo, err := repository.New(pool)
	if err != nil {
		t.Fatalf("repository.New() error = %v", err)
	}

	parcel := newFakeParcel()
	parcelServer := parcel.server()
	t.Cleanup(parcelServer.Close)
	parcelClient, err := parcelclient.New(parcelServer.URL, "internal-token", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("parcelclient.New() error = %v", err)
	}
	identityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"role":"MERCHANT","isActive":true,"sessionVersion":1}}`)
	}))
	t.Cleanup(identityServer.Close)
	identity, err := identityclient.New(identityServer.URL, "", time.Second)
	if err != nil {
		t.Fatalf("identityclient.New() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewWithOptions(Options{
		Repository: repo, Identity: identity,
		Orders:       orders.New(repo, parcelClient, staticZones{name: "Colombo"}),
		AccessSecret: testJWTSecret, CookieName: "sd_access", InternalToken: "internal-token", Logger: logger,
	})
	return ctx, repo, pool, router, parcel
}

func testHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}

func createHTTPMerchant(t *testing.T, ctx context.Context, repo *repository.Repository) (merchantID, ownerID string) {
	t.Helper()
	merchantID, ownerID = userID(t), userID(t)
	if err := repo.CreateMerchant(ctx, repository.CreateMerchantParams{
		ID: merchantID, OwnerUserID: ownerID, BusinessName: "Order API " + merchantID[:8],
		BusinessRegNo: "API-" + merchantID[:8], ContactEmail: "api-" + merchantID[:8] + "@example.test",
		ContactPhone: "+94771234567", PickupAddress: "12 Galle Road", PickupZoneID: userID(t),
	}); err != nil {
		t.Fatalf("CreateMerchant() error = %v", err)
	}
	return merchantID, ownerID
}

func requestJSON(t *testing.T, router http.Handler, method, path string, body any, headers map[string]string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.Bytes()
}

func orderBody(external string) map[string]any {
	return map[string]any{
		"externalOrderId": external,
		"recipient": map[string]any{
			"name": "Amali Fernando", "phone": "+94712345678", "address": "45 Lake Drive",
			"city": "Kandy", "zoneId": "44444444-4444-4444-8444-444444444444",
		},
		"weightKg": 1.5, "sizeCategory": "SMALL", "description": "Phone case",
	}
}

func TestMerchantOrderAPIKeyCreateReadReplayAndIsolation(t *testing.T) {
	ctx, repo, _, router, parcel := newOrderHTTPFixture(t)
	merchantID, _ := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, merchantID)
	key := newKey(t, ctx, repo, merchantID, "orders")
	headers := map[string]string{"X-API-Key": key.full, "Idempotency-Key": "key-1"}

	status, body := requestJSON(t, router, http.MethodPost, "/merchant/orders", orderBody("SZ-100245"), headers)
	if status != http.StatusCreated {
		t.Fatalf("create status = %d (%s)", status, body)
	}
	var created struct {
		Data struct {
			OrderID, ExternalOrderID, TrackingNumber, Status string
			Fee                                              orders.Fee
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Data.OrderID == "" || created.Data.TrackingNumber == "" || created.Data.Fee.Total != 55000 || created.Data.Status != "CREATED" {
		t.Fatalf("create body = %s", body)
	}

	status, body = requestJSON(t, router, http.MethodPost, "/merchant/orders", orderBody("SZ-100245"), headers)
	if status != http.StatusOK || !bytes.Contains(body, []byte(created.Data.OrderID)) {
		t.Fatalf("replay status = %d (%s)", status, body)
	}
	if parcel.calls != 1 {
		t.Fatalf("parcel calls = %d, want 1", parcel.calls)
	}

	different := orderBody("SZ-100245")
	different["recipient"].(map[string]any)["name"] = "Someone Else"
	status, body = requestJSON(t, router, http.MethodPost, "/merchant/orders", different, headers)
	if status != http.StatusConflict || errorCode(body) != "DUPLICATE_ORDER" {
		t.Fatalf("conflict status = %d (%s)", status, body)
	}

	status, body = requestJSON(t, router, http.MethodGet, "/merchant/orders", nil, map[string]string{"X-API-Key": key.full})
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"totalItems":1`)) {
		t.Fatalf("list status = %d (%s)", status, body)
	}
	status, body = requestJSON(t, router, http.MethodGet, "/merchant/orders/"+created.Data.OrderID, nil, map[string]string{"X-API-Key": key.full})
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"timeline"`)) || !bytes.Contains(body, []byte(`"weightKg":1.50`)) {
		t.Fatalf("detail status = %d (%s)", status, body)
	}

	otherID, _ := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, otherID)
	otherKey := newKey(t, ctx, repo, otherID, "other")
	status, body = requestJSON(t, router, http.MethodGet, "/merchant/orders/"+created.Data.OrderID, nil, map[string]string{"X-API-Key": otherKey.full})
	if status != http.StatusNotFound || errorCode(body) != "NOT_FOUND" {
		t.Fatalf("cross-merchant status = %d (%s)", status, body)
	}
	if bytes.Contains(body, []byte(created.Data.TrackingNumber)) {
		t.Fatalf("cross-merchant response leaked the tracking number: %s", body)
	}
}

func TestMerchantOrderAPIRejectsInvalidAndUnapproved(t *testing.T) {
	ctx, repo, _, router, _ := newOrderHTTPFixture(t)
	pendingID, _ := createHTTPMerchant(t, ctx, repo)
	pendingKey := newKey(t, ctx, repo, pendingID, "pending")
	status, body := requestJSON(t, router, http.MethodPost, "/merchant/orders", orderBody("SZ-PENDING"), map[string]string{"X-API-Key": pendingKey.full})
	if status != http.StatusForbidden || errorCode(body) != "MERCHANT_NOT_APPROVED" {
		t.Fatalf("pending status = %d (%s)", status, body)
	}

	approvedID, _ := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, approvedID)
	key := newKey(t, ctx, repo, approvedID, "approved")
	headers := map[string]string{"X-API-Key": key.full}

	invalid := orderBody("SZ-INVALID")
	invalid["recipient"].(map[string]any)["phone"] = "0712345678"
	status, body = requestJSON(t, router, http.MethodPost, "/merchant/orders", invalid, headers)
	if status != http.StatusUnprocessableEntity || errorCode(body) != "VALIDATION_ERROR" {
		t.Fatalf("invalid status = %d (%s)", status, body)
	}

	unknown := orderBody("SZ-UNKNOWN")
	unknown["merchantId"] = "99999999-9999-4999-8999-999999999999"
	status, body = requestJSON(t, router, http.MethodPost, "/merchant/orders", unknown, headers)
	if status != http.StatusBadRequest || errorCode(body) != "BAD_REQUEST" {
		t.Fatalf("unknown field status = %d (%s)", status, body)
	}

	status, body = requestJSON(t, router, http.MethodPost, "/merchant/orders", orderBody("SZ-ROUTE-FAIL"), headers)
	if status != http.StatusUnprocessableEntity || errorCode(body) != "ROUTE_NOT_SERVICEABLE" {
		t.Fatalf("pricing failure status = %d (%s)", status, body)
	}
}

func TestMerchantOrderSessionReadAndApprovalGate(t *testing.T) {
	ctx, repo, _, router, _ := newOrderHTTPFixture(t)
	merchantID, ownerID := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, merchantID)
	key := newKey(t, ctx, repo, merchantID, "session")
	status, body := requestJSON(t, router, http.MethodPost, "/merchant/orders", orderBody("SZ-SESSION"), map[string]string{"X-API-Key": key.full})
	if status != http.StatusCreated {
		t.Fatalf("create status = %d (%s)", status, body)
	}
	var created struct {
		Data struct{ OrderID string } `json:"data"`
	}
	_ = json.Unmarshal(body, &created)

	manager, err := auth.NewTokenManager(testJWTSecret, time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	token, _, err := manager.IssueWithSession(ownerID, auth.RoleMerchant, 1)
	if err != nil {
		t.Fatalf("IssueWithSession() error = %v", err)
	}
	status, body = requestJSON(t, router, http.MethodGet, "/merchant/orders/"+created.Data.OrderID, nil, map[string]string{"Authorization": "Bearer " + token})
	if status != http.StatusOK || !bytes.Contains(body, []byte("SZ-SESSION")) {
		t.Fatalf("session detail status = %d (%s)", status, body)
	}

	if _, err := repo.SuspendMerchant(ctx, merchantID, userID(t), "review"); err != nil {
		t.Fatalf("SuspendMerchant() error = %v", err)
	}
	status, body = requestJSON(t, router, http.MethodGet, "/merchant/orders/"+created.Data.OrderID, nil, map[string]string{"Authorization": "Bearer " + token})
	if status != http.StatusForbidden || errorCode(body) != "MERCHANT_NOT_APPROVED" {
		t.Fatalf("suspended session status = %d (%s)", status, body)
	}
}

func TestMerchantOrderConcurrentDuplicateCreatesOneOrder(t *testing.T) {
	ctx, repo, _, router, parcel := newOrderHTTPFixture(t)
	merchantID, _ := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, merchantID)
	key := newKey(t, ctx, repo, merchantID, "concurrent")
	headers := map[string]string{"X-API-Key": key.full, "Idempotency-Key": "same-key"}

	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _ := requestJSON(t, router, http.MethodPost, "/merchant/orders", orderBody("SZ-CONCURRENT"), headers)
			statuses <- status
		}()
	}
	wg.Wait()
	close(statuses)
	created := 0
	replayed := 0
	for status := range statuses {
		switch status {
		case http.StatusCreated:
			created++
		case http.StatusOK:
			replayed++
		default:
			t.Fatalf("concurrent status = %d", status)
		}
	}
	if created != 1 || replayed != 1 {
		t.Fatalf("created=%d replayed=%d, want 1/1", created, replayed)
	}
	if parcel.calls != 1 {
		t.Fatalf("parcel calls = %d, want 1", parcel.calls)
	}
}

func TestMerchantOrderTimeoutDoesNotPersistThenRetrySucceeds(t *testing.T) {
	ctx, repo, _, router, _ := newOrderHTTPFixture(t)
	merchantID, _ := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, merchantID)
	key := newKey(t, ctx, repo, merchantID, "timeout")
	headers := map[string]string{"X-API-Key": key.full}

	status, body := requestJSON(t, router, http.MethodPost, "/merchant/orders", orderBody("SZ-TIMEOUT"), headers)
	if status != http.StatusServiceUnavailable || errorCode(body) != "SERVICE_UNAVAILABLE" {
		t.Fatalf("timeout status = %d (%s)", status, body)
	}
	status, body = requestJSON(t, router, http.MethodGet, "/merchant/orders", nil, headers)
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"totalItems":0`)) {
		t.Fatalf("list after timeout = %d (%s)", status, body)
	}
	status, body = requestJSON(t, router, http.MethodPost, "/merchant/orders", orderBody("SZ-TIMEOUT"), headers)
	if status != http.StatusCreated {
		t.Fatalf("retry status = %d (%s)", status, body)
	}
}

func TestMerchantOrderRateLimit(t *testing.T) {
	ctx, repo, _, router, _ := newOrderHTTPFixture(t)
	merchantID, _ := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, merchantID)
	key := newKey(t, ctx, repo, merchantID, "rate")
	headers := map[string]string{"X-API-Key": key.full}
	body := orderBody("SZ-RATE")
	for i := 0; i < 120; i++ {
		status, _ := requestJSON(t, router, http.MethodPost, "/merchant/orders", body, headers)
		if status != http.StatusCreated && status != http.StatusOK {
			t.Fatalf("request %d status = %d", i+1, status)
		}
	}
	status, payload := requestJSON(t, router, http.MethodPost, "/merchant/orders", body, headers)
	if status != http.StatusTooManyRequests || errorCode(payload) != "RATE_LIMITED" {
		t.Fatalf("rate-limit status = %d (%s)", status, payload)
	}
}

func TestMerchantAPIKeyLimitIsConcurrencySafe(t *testing.T) {
	ctx, repo, _, _, _ := newOrderHTTPFixture(t)
	merchantID, _ := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, merchantID)

	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			key, err := apikey.Generate()
			if err != nil {
				results <- err
				return
			}
			err = repo.CreateAPIClient(ctx, repository.CreateAPIClientParams{
				ID: urlUUID(), MerchantID: merchantID, Name: fmt.Sprintf("key-%d", index),
				KeyPrefix: key.KeyPrefix, LookupID: key.LookupID, KeyHash: key.Hash,
			})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	created, limited := 0, 0
	for err := range results {
		switch {
		case err == nil:
			created++
		case isKeyLimit(err):
			limited++
		default:
			t.Fatalf("CreateAPIClient() error = %v", err)
		}
	}
	if created != 5 || limited != 5 {
		t.Fatalf("created=%d limited=%d, want 5/5", created, limited)
	}
}

func isKeyLimit(err error) bool {
	return errors.Is(err, repository.ErrKeyLimit) || (err != nil && bytes.Contains([]byte(err.Error()), []byte("KEY_LIMIT_REACHED")))
}

func urlUUID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}
