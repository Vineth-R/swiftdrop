package servicehttp

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseMerchantAuditQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		query  string
		status int
	}{
		{"valid", "?page=2&pageSize=10&actorUserId=11111111-1111-4111-8111-111111111111&action=APPROVED&sort=createdAt:asc", 200},
		{"invalid page", "?page=0", 422},
		{"invalid page size", "?pageSize=101", 422},
		{"invalid actor", "?actorUserId=nope", 422},
		{"invalid date range", "?from=2026-11-03&to=2026-11-02", 422},
		{"invalid sort", "?sort=createdAt:nope", 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest("GET", "/admin/audit-logs/merchant"+test.query, nil)
			_, ok := parseMerchantAuditQuery(context)
			got := 422
			if ok {
				got = 200
			}
			if got != test.status {
				t.Fatalf("status = %d, want %d", got, test.status)
			}
		})
	}
}
