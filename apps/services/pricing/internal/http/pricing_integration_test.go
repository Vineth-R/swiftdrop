//go:build integration

package servicehttp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/pricing/internal/pricing"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const pricingIntegrationTimeout = 120 * time.Second

// newPricingTestDatabase creates a uniquely named database on the server named
// by TEST_DATABASE_URL, applies the pricing migrations, and drops it on cleanup.
// It never connects to or modifies the running application database.
func newPricingTestDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping PostgreSQL pricing HTTP integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), pricingIntegrationTimeout)
	t.Cleanup(cancel)

	adminConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_pricing_http_test_" + pricingRandomHex(8)
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

	testConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL for isolated database: %v", err)
	}
	testConfig.Database = databaseName
	conn, err := pgx.ConnectConfig(ctx, testConfig)
	if err != nil {
		t.Fatalf("connect to isolated database: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close(context.Background())
	})

	applyPricingSQLFile(t, ctx, conn, "infra/docker/postgres/init/01-schemas.sql")
	applyPricingSQLFile(t, ctx, conn, "migrations/pricing/000001_initial_schema.up.sql")
	applyPricingSQLFile(t, ctx, conn, "migrations/pricing/000002_tables.up.sql")

	return ctx, conn
}

func applyPricingSQLFile(t *testing.T, ctx context.Context, conn *pgx.Conn, relativePath string) {
	t.Helper()
	root := filepath.Join("..", "..", "..", "..", "..")
	content, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	if _, err := conn.PgConn().Exec(ctx, string(content)).ReadAll(); err != nil {
		t.Fatalf("apply %s: %v", relativePath, err)
	}
}

func pricingRandomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return hex.EncodeToString(buffer)
}

func pricingZoneID(t *testing.T, ctx context.Context, conn *pgx.Conn, code string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, "SELECT id::text FROM pricing.zones WHERE code=$1", code).Scan(&id); err != nil {
		t.Fatalf("read zone %s: %v", code, err)
	}
	return id
}

func newPricingIntegrationRouter(t *testing.T, conn *pgx.Conn) *gin.Engine {
	t.Helper()
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: pricing.NewRepository(conn)})
	if err != nil {
		t.Fatalf("build pricing router: %v", err)
	}
	return router
}

func pricingQuoteBody(origin, destination, weight, size string) string {
	return fmt.Sprintf(`{"originZoneId":%q,"destinationZoneId":%q,"weightKg":%s,"sizeCategory":%q}`,
		origin, destination, weight, size)
}

func pricingCall(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeQuote(t *testing.T, recorder *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var response struct {
		Data struct {
			BaseFee       json.Number `json:"baseFee"`
			WeightCharge  json.Number `json:"weightCharge"`
			SizeSurcharge json.Number `json:"sizeSurcharge"`
			Total         json.Number `json:"total"`
			Currency      string      `json:"currency"`
			WeightKg      json.Number `json:"weightKg"`
			SizeCategory  string      `json:"sizeCategory"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(strings.NewReader(recorder.Body.String()))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode quote response: %v; body %s", err, recorder.Body.String())
	}
	return map[string]string{
		"baseFee":       response.Data.BaseFee.String(),
		"weightCharge":  response.Data.WeightCharge.String(),
		"sizeSurcharge": response.Data.SizeSurcharge.String(),
		"total":         response.Data.Total.String(),
		"currency":      response.Data.Currency,
		"weightKg":      response.Data.WeightKg.String(),
		"sizeCategory":  response.Data.SizeCategory,
	}
}

func decodeErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v; body %s", err, recorder.Body.String())
	}
	return response.Error.Code
}

func TestPricingHTTPQuoteAgainstDatabase(t *testing.T) {
	ctx, conn := newPricingTestDatabase(t)
	origin := pricingZoneID(t, ctx, conn, "CMB")
	destination := pricingZoneID(t, ctx, conn, "KDY")
	router := newPricingIntegrationRouter(t, conn)

	quote := func(t *testing.T, weight, size string) map[string]string {
		t.Helper()
		recorder := pricingCall(router, http.MethodPost, "/pricing/estimate", pricingQuoteBody(origin, destination, weight, size))
		if recorder.Code != http.StatusOK {
			t.Fatalf("weight %s size %s: status %d, body %s", weight, size, recorder.Code, recorder.Body.String())
		}
		return decodeQuote(t, recorder)
	}

	t.Run("seed 550", func(t *testing.T) {
		got := quote(t, "2", "SMALL")
		want := map[string]string{
			"baseFee": "450.00", "weightCharge": "100.00", "sizeSurcharge": "0.00",
			"total": "550.00", "currency": "LKR", "weightKg": "2.00", "sizeCategory": "SMALL",
		}
		for field, value := range want {
			if got[field] != value {
				t.Fatalf("%s = %q, want %q (quote %v)", field, got[field], value, got)
			}
		}
	})

	for _, test := range []struct {
		name, weight, wantWeightCharge, wantTotal string
	}{
		{name: "1 kg lower boundary", weight: "1", wantWeightCharge: "0.00", wantTotal: "450.00"},
		{name: "5 kg upper boundary", weight: "5", wantWeightCharge: "100.00", wantTotal: "550.00"},
		{name: "5.01 kg next band", weight: "5.01", wantWeightCharge: "250.00", wantTotal: "700.00"},
		{name: "10 kg upper boundary", weight: "10", wantWeightCharge: "250.00", wantTotal: "700.00"},
		{name: "10.01 kg next band", weight: "10.01", wantWeightCharge: "500.00", wantTotal: "950.00"},
		{name: "25 kg maximum boundary", weight: "25", wantWeightCharge: "500.00", wantTotal: "950.00"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := quote(t, test.weight, "SMALL")
			if got["weightCharge"] != test.wantWeightCharge || got["total"] != test.wantTotal {
				t.Fatalf("quote %v, want weightCharge %s total %s", got, test.wantWeightCharge, test.wantTotal)
			}
		})
	}

	for _, test := range []struct{ size, wantSurcharge, wantTotal string }{
		{size: "SMALL", wantSurcharge: "0.00", wantTotal: "550.00"},
		{size: "MEDIUM", wantSurcharge: "150.00", wantTotal: "700.00"},
		{size: "LARGE", wantSurcharge: "400.00", wantTotal: "950.00"},
		{size: "OVERSIZED", wantSurcharge: "900.00", wantTotal: "1450.00"},
	} {
		t.Run("size "+test.size, func(t *testing.T) {
			got := quote(t, "2", test.size)
			if got["sizeSurcharge"] != test.wantSurcharge || got["total"] != test.wantTotal {
				t.Fatalf("quote %v, want surcharge %s total %s", got, test.wantSurcharge, test.wantTotal)
			}
		})
	}

	t.Run("above maximum", func(t *testing.T) {
		recorder := pricingCall(router, http.MethodPost, "/pricing/estimate", pricingQuoteBody(origin, destination, "25.01", "SMALL"))
		if recorder.Code != http.StatusUnprocessableEntity || decodeErrorCode(t, recorder) != "WEIGHT_NOT_PRICED" {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		for _, body := range []string{
			pricingQuoteBody(origin, destination, "0", "SMALL"),
			pricingQuoteBody(origin, destination, "-1", "SMALL"),
			pricingQuoteBody(origin, destination, "0.001", "SMALL"),
			pricingQuoteBody(origin, destination, "2", "XL"),
		} {
			recorder := pricingCall(router, http.MethodPost, "/pricing/estimate", body)
			if recorder.Code != http.StatusUnprocessableEntity || decodeErrorCode(t, recorder) != "VALIDATION_ERROR" {
				t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
			}
		}
	})

	t.Run("unknown zone", func(t *testing.T) {
		unknown := "99999999-9999-4999-8999-999999999999"
		recorder := pricingCall(router, http.MethodPost, "/pricing/estimate", pricingQuoteBody(unknown, destination, "2", "SMALL"))
		if recorder.Code != http.StatusNotFound || decodeErrorCode(t, recorder) != "ZONE_NOT_FOUND" {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("gap between active bands", func(t *testing.T) {
		if _, err := conn.Exec(ctx, "UPDATE pricing.weight_bands SET is_active=false WHERE min_weight_kg=1 AND max_weight_kg=5"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = conn.Exec(context.Background(), "UPDATE pricing.weight_bands SET is_active=true WHERE min_weight_kg=1 AND max_weight_kg=5")
		})
		recorder := pricingCall(router, http.MethodPost, "/pricing/estimate", pricingQuoteBody(origin, destination, "2", "SMALL"))
		if recorder.Code != http.StatusUnprocessableEntity || decodeErrorCode(t, recorder) != "WEIGHT_NOT_PRICED" {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("unpriced route takes precedence", func(t *testing.T) {
		if _, err := conn.Exec(ctx, "UPDATE pricing.pricing_rules SET is_active=false WHERE origin_zone_id=$1 AND destination_zone_id=$2", origin, destination); err != nil {
			t.Fatal(err)
		}
		for _, weight := range []string{"2", "30"} {
			recorder := pricingCall(router, http.MethodPost, "/pricing/estimate", pricingQuoteBody(origin, destination, weight, "SMALL"))
			if recorder.Code != http.StatusUnprocessableEntity || decodeErrorCode(t, recorder) != "ROUTE_NOT_SERVICEABLE" {
				t.Fatalf("weight %s: status %d, body %s", weight, recorder.Code, recorder.Body.String())
			}
		}
	})
}

func TestPricingHTTPZonesAgainstDatabase(t *testing.T) {
	ctx, conn := newPricingTestDatabase(t)
	router := newPricingIntegrationRouter(t, conn)

	t.Run("active zones", func(t *testing.T) {
		recorder := pricingCall(router, http.MethodGet, "/zones", "")
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"code":"CMB"`) ||
			!strings.Contains(recorder.Body.String(), `"code":"KDY"`) || strings.Contains(recorder.Body.String(), "isActive") {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("inactive zones excluded", func(t *testing.T) {
		if _, err := conn.Exec(ctx, "UPDATE pricing.zones SET is_active=false WHERE code='KDY'"); err != nil {
			t.Fatal(err)
		}
		recorder := pricingCall(router, http.MethodGet, "/zones", "")
		if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"code":"KDY"`) {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
		origin := pricingZoneID(t, ctx, conn, "CMB")
		destination := pricingZoneID(t, ctx, conn, "KDY")
		quoteRecorder := pricingCall(router, http.MethodPost, "/pricing/estimate", pricingQuoteBody(origin, destination, "2", "SMALL"))
		if quoteRecorder.Code != http.StatusNotFound || decodeErrorCode(t, quoteRecorder) != "ZONE_NOT_FOUND" {
			t.Fatalf("status %d, body %s", quoteRecorder.Code, quoteRecorder.Body.String())
		}
	})

	t.Run("empty result", func(t *testing.T) {
		if _, err := conn.Exec(ctx, "UPDATE pricing.zones SET is_active=false"); err != nil {
			t.Fatal(err)
		}
		recorder := pricingCall(router, http.MethodGet, "/zones", "")
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"data":[]`) {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		// Closing the connection produces a real database failure without
		// replacing the repository with a fake.
		if err := conn.Close(ctx); err != nil {
			t.Fatal(err)
		}
		failedRouter := newPricingIntegrationRouter(t, conn)
		recorder := pricingCall(failedRouter, http.MethodGet, "/zones", "")
		if recorder.Code != http.StatusInternalServerError || decodeErrorCode(t, recorder) != "INTERNAL_ERROR" {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestPricingHTTPAdminChangesAgainstDatabase(t *testing.T) {
	ctx, conn := newPricingTestDatabase(t)
	origin := pricingZoneID(t, ctx, conn, "CMB")
	destination := pricingZoneID(t, ctx, conn, "KDY")
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: pricing.NewRepository(conn), AccessSecret: testSecret})
	if err != nil {
		t.Fatalf("build pricing router: %v", err)
	}
	token := adminToken(t, auth.RoleAdmin)
	admin := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	for _, test := range []struct{ name, path, body, code string }{
		{name: "overlapping band", path: "/admin/pricing/bands", body: `{"minWeightKg":4,"maxWeightKg":6,"charge":1}`, code: "BAND_OVERLAP"},
		{name: "duplicate zone code", path: "/admin/zones", body: `{"code":"CMB","name":"Duplicate"}`, code: "ZONE_CODE_EXISTS"},
		{name: "duplicate route", path: "/admin/pricing/rules", body: fmt.Sprintf(`{"originZoneId":%q,"destinationZoneId":%q,"baseFee":1}`, origin, destination), code: "RULE_EXISTS"},
	} {
		t.Run(test.name+" refused", func(t *testing.T) {
			recorder := admin(http.MethodPost, test.path, test.body)
			if recorder.Code != http.StatusConflict || decodeErrorCode(t, recorder) != test.code {
				t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
			}
		})
	}

	// The lower bound is exclusive, so a band may start where the last one ends.
	t.Run("touching band accepted and priced", func(t *testing.T) {
		recorder := admin(http.MethodPost, "/admin/pricing/bands", `{"minWeightKg":25,"maxWeightKg":30,"charge":700}`)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
		recorder = pricingCall(router, http.MethodPost, "/pricing/estimate", pricingQuoteBody(origin, destination, "30", "SMALL"))
		if recorder.Code != http.StatusOK || decodeQuote(t, recorder)["weightCharge"] != "700.00" {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("rule change gives a new estimate", func(t *testing.T) {
		var rule string
		if err := conn.QueryRow(ctx, "SELECT id::text FROM pricing.pricing_rules WHERE origin_zone_id=$1 AND destination_zone_id=$2 AND is_active", origin, destination).Scan(&rule); err != nil {
			t.Fatal(err)
		}
		if recorder := admin(http.MethodPatch, "/admin/pricing/rules/"+rule, `{"baseFee":500}`); recorder.Code != http.StatusOK {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
		recorder := pricingCall(router, http.MethodPost, "/pricing/estimate", pricingQuoteBody(origin, destination, "2", "SMALL"))
		if recorder.Code != http.StatusOK || decodeQuote(t, recorder)["total"] != "600.00" {
			t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
		}
	})
}
