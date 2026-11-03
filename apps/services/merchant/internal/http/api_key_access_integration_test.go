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
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/apikey"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

// These tests prove the cross-service access rule for merchant API keys (API
// v1.1 section 6.2): a key works only while it is ACTIVE and its merchant is
// APPROVED. They run the real MerchantAPIKey middleware against the real merchant
// schema in a disposable database on TEST_DATABASE_URL.

const keyAccessTimeout = 120 * time.Second

func TestMerchantAPIKey_access(t *testing.T) {
	ctx, repo, probe := newKeyAccessFixture(t)

	approved := newMerchant(t, ctx, repo)
	approve(t, ctx, repo, approved)
	activeKey := newKey(t, ctx, repo, approved, "active")
	revokedKey := newKey(t, ctx, repo, approved, "to be revoked")
	if err := repo.RevokeAPIClient(ctx, approved, revokedKey.id); err != nil {
		t.Fatalf("RevokeAPIClient returned error: %v", err)
	}

	other := newMerchant(t, ctx, repo)
	approve(t, ctx, repo, other)
	otherKey := newKey(t, ctx, repo, other, "other merchant")

	pending := newMerchant(t, ctx, repo)
	pendingKey := newKey(t, ctx, repo, pending, "pending merchant")

	rejected := newMerchant(t, ctx, repo)
	if _, err := repo.RejectMerchant(ctx, rejected, userID(t), "documents do not match"); err != nil {
		t.Fatalf("RejectMerchant returned error: %v", err)
	}
	rejectedKey := newKey(t, ctx, repo, rejected, "rejected merchant")

	suspended := newMerchant(t, ctx, repo)
	approve(t, ctx, repo, suspended)
	suspendedKey := newKey(t, ctx, repo, suspended, "suspended merchant")
	if _, err := repo.SuspendMerchant(ctx, suspended, userID(t), "chargebacks"); err != nil {
		t.Fatalf("SuspendMerchant returned error: %v", err)
	}

	wrongSecret := activeKey.full[:len(activeKey.full)-4] + "0000"
	if wrongSecret == activeKey.full {
		wrongSecret = activeKey.full[:len(activeKey.full)-4] + "ffff"
	}
	unknownLookup := apikey.Prefix + strings.Repeat("0", apikey.LookupLength) + "_" + strings.Repeat("a", apikey.SecretBytes*2)

	for _, test := range []struct {
		name       string
		key        string
		wantStatus int
		wantCode   string
		wantMerch  string
	}{
		{"an active key of an approved merchant works", activeKey.full, http.StatusOK, "", approved},
		{"a key resolves only to its own merchant", otherKey.full, http.StatusOK, "", other},
		{"a revoked key is refused", revokedKey.full, http.StatusUnauthorized, "INVALID_API_KEY", ""},
		{"a key of a suspended merchant is refused", suspendedKey.full, http.StatusForbidden, "MERCHANT_NOT_APPROVED", ""},
		{"a key of a pending merchant is refused", pendingKey.full, http.StatusForbidden, "MERCHANT_NOT_APPROVED", ""},
		{"a key of a rejected merchant is refused", rejectedKey.full, http.StatusForbidden, "MERCHANT_NOT_APPROVED", ""},
		{"a key with the wrong secret is refused", wrongSecret, http.StatusUnauthorized, "INVALID_API_KEY", ""},
		{"a key that was never issued is refused", unknownLookup, http.StatusUnauthorized, "INVALID_API_KEY", ""},
		{"a malformed key is refused", "not-a-key", http.StatusUnauthorized, "INVALID_API_KEY", ""},
		{"a missing key is refused", "", http.StatusUnauthorized, "INVALID_API_KEY", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, body := probe(test.key)
			if status != test.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", status, test.wantStatus, body)
			}
			if test.wantCode != "" && errorCode(body) != test.wantCode {
				t.Fatalf("error code = %q, want %q (body %s)", errorCode(body), test.wantCode, body)
			}
			if test.wantMerch != "" && merchantIDOf(body) != test.wantMerch {
				t.Fatalf("merchant = %q, want %q", merchantIDOf(body), test.wantMerch)
			}
			if test.wantStatus != http.StatusOK && strings.Contains(string(body), "merchantId") {
				t.Fatalf("a refused request still named a merchant: %s", body)
			}
		})
	}

	t.Run("revoking one key leaves the merchant's other keys working", func(t *testing.T) {
		if status, _ := probe(activeKey.full); status != http.StatusOK {
			t.Fatalf("the remaining key stopped working: status %d", status)
		}
	})

	t.Run("a key stops working the moment its merchant is suspended", func(t *testing.T) {
		live := newMerchant(t, ctx, repo)
		approve(t, ctx, repo, live)
		key := newKey(t, ctx, repo, live, "soon suspended")
		if status, _ := probe(key.full); status != http.StatusOK {
			t.Fatalf("status before suspension = %d, want 200", status)
		}
		if _, err := repo.SuspendMerchant(ctx, live, userID(t), "fraud review"); err != nil {
			t.Fatalf("SuspendMerchant returned error: %v", err)
		}
		if status, body := probe(key.full); status != http.StatusForbidden || errorCode(body) != "MERCHANT_NOT_APPROVED" {
			t.Fatalf("status after suspension = %d (%s), want 403 MERCHANT_NOT_APPROVED", status, body)
		}
	})

	t.Run("a key stops working the moment it is revoked", func(t *testing.T) {
		live := newMerchant(t, ctx, repo)
		approve(t, ctx, repo, live)
		key := newKey(t, ctx, repo, live, "soon revoked")
		if status, _ := probe(key.full); status != http.StatusOK {
			t.Fatalf("status before revocation = %d, want 200", status)
		}
		if err := repo.RevokeAPIClient(ctx, live, key.id); err != nil {
			t.Fatalf("RevokeAPIClient returned error: %v", err)
		}
		if status, body := probe(key.full); status != http.StatusUnauthorized || errorCode(body) != "INVALID_API_KEY" {
			t.Fatalf("status after revocation = %d (%s), want 401 INVALID_API_KEY", status, body)
		}
	})
}

type issuedKey struct {
	id   string
	full string
}

func newKeyAccessFixture(t *testing.T) (context.Context, *repository.Repository, func(key string) (int, []byte)) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping merchant API key access tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), keyAccessTimeout)
	t.Cleanup(cancel)

	adminConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_merchant_access_test_" + randomHex(8)
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
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	root := filepath.Join("..", "..", "..", "..", "..")
	applySQL(t, ctx, conn, filepath.Join(root, "infra/docker/postgres/init/01-schemas.sql"))
	migrations, err := filepath.Glob(filepath.Join(root, "migrations/merchant/*.up.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find merchant migrations: %v", err)
	}
	sort.Strings(migrations)
	for _, migration := range migrations {
		applySQL(t, ctx, conn, migration)
	}

	repo, err := repository.New(conn)
	if err != nil {
		t.Fatalf("repository.New returned error: %v", err)
	}

	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpx.NewRouter(logger)
	router.GET("/probe", MerchantAPIKey(repo, logger), func(c *gin.Context) {
		identity, _ := identityFrom(c)
		c.JSON(http.StatusOK, gin.H{"merchantId": identity.MerchantID, "role": identity.Role})
	})

	probe := func(key string) (int, []byte) {
		request := httptest.NewRequest(http.MethodGet, "/probe", nil)
		if key != "" {
			request.Header.Set("X-API-Key", key)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder.Code, recorder.Body.Bytes()
	}
	return ctx, repo, probe
}

func applySQL(t *testing.T, ctx context.Context, conn *pgx.Conn, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := conn.PgConn().Exec(ctx, string(content)).ReadAll(); err != nil {
		t.Fatalf("apply %s: %v", path, err)
	}
}

func newMerchant(t *testing.T, ctx context.Context, repo *repository.Repository) string {
	t.Helper()
	id := userID(t)
	if err := repo.CreateMerchant(ctx, repository.CreateMerchantParams{
		ID:            id,
		OwnerUserID:   userID(t),
		BusinessName:  "Access Test " + id[:8],
		ContactEmail:  "owner-" + id[:8] + "@example.test",
		ContactPhone:  "+94771234567",
		PickupAddress: "12 Galle Road, Colombo",
		PickupZoneID:  userID(t),
	}); err != nil {
		t.Fatalf("CreateMerchant returned error: %v", err)
	}
	return id
}

func approve(t *testing.T, ctx context.Context, repo *repository.Repository, merchantID string) {
	t.Helper()
	if _, err := repo.ApproveMerchant(ctx, merchantID, userID(t)); err != nil {
		t.Fatalf("ApproveMerchant returned error: %v", err)
	}
}

func newKey(t *testing.T, ctx context.Context, repo *repository.Repository, merchantID, name string) issuedKey {
	t.Helper()
	key, err := apikey.Generate()
	if err != nil {
		t.Fatalf("apikey.Generate returned error: %v", err)
	}
	id := userID(t)
	if err := repo.CreateAPIClient(ctx, repository.CreateAPIClientParams{
		ID: id, MerchantID: merchantID, Name: name, KeyPrefix: key.KeyPrefix, LookupID: key.LookupID, KeyHash: key.Hash,
	}); err != nil {
		t.Fatalf("CreateAPIClient returned error: %v", err)
	}
	return issuedKey{id: id, full: key.Full}
}

// userID returns a random version 4 UUID. The merchant schema keeps user and
// zone IDs as plain UUIDs without foreign keys, so any value is acceptable.
func userID(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return hex.EncodeToString(buffer)
}

func errorCode(body []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	return envelope.Error.Code
}

func merchantIDOf(body []byte) string {
	var envelope struct {
		MerchantID string `json:"merchantId"`
	}
	_ = json.Unmarshal(body, &envelope)
	return envelope.MerchantID
}
