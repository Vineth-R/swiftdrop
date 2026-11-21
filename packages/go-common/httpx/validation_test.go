package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

func TestErrorItemsPreservesOrder(t *testing.T) {
	issues := []validation.Issue{
		{Field: "email", Issue: "must be a valid email address"},
		{Field: "password", Issue: "must be at least 8 characters"},
	}
	items := ErrorItems(issues)
	if len(items) != 2 || items[0].Field != "email" || items[1].Issue != "must be at least 8 characters" {
		t.Fatalf("ErrorItems() = %+v", items)
	}
	if ErrorItems(nil) != nil {
		t.Fatal("ErrorItems(nil) should be nil")
	}
}

func TestErrorWithDetailsIncludesValidationDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(discardLogger())
	router.GET("/invalid", func(c *gin.Context) {
		ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.",
			ErrorItems([]validation.Issue{{Field: "page", Issue: "must be between 1 and 10000"}}))
	})

	request := httptest.NewRequest(http.MethodGet, "/invalid", nil)
	request.Header.Set(requestid.Header, "req-validation")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	var body ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != "VALIDATION_ERROR" || body.Error.Message != "One or more fields are invalid." {
		t.Fatalf("error = %+v", body.Error)
	}
	if len(body.Error.Details) != 1 || body.Error.Details[0].Field != "page" {
		t.Fatalf("details = %+v", body.Error.Details)
	}
	if body.Meta.RequestID != "req-validation" {
		t.Fatalf("requestId = %q", body.Meta.RequestID)
	}
}
