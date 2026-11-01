package httpx

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

type envelopeBody struct {
	Data json.RawMessage `json:"data"`
	Meta struct {
		RequestID  string `json:"requestId"`
		Timestamp  string `json:"timestamp"`
		Pagination *struct {
			Page       int `json:"page"`
			PageSize   int `json:"pageSize"`
			TotalItems int `json:"totalItems"`
			TotalPages int `json:"totalPages"`
		} `json:"pagination"`
	} `json:"meta"`
}

func serve(t *testing.T, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	router.GET("/x", handler)

	request := httptest.NewRequest(http.MethodGet, "/x", nil)
	request.Header.Set(requestid.Header, "req-123")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestNewPagination(t *testing.T) {
	tests := []struct {
		name              string
		page, size, items int
		wantPages         int
	}{
		{name: "exact multiple", page: 1, size: 20, items: 40, wantPages: 2},
		{name: "partial last page", page: 3, size: 20, items: 57, wantPages: 3},
		{name: "empty result", page: 1, size: 20, items: 0, wantPages: 0},
		{name: "one item", page: 1, size: 20, items: 1, wantPages: 1},
		{name: "zero page size", page: 1, size: 0, items: 10, wantPages: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NewPagination(test.page, test.size, test.items)
			if got.TotalPages != test.wantPages || got.Page != test.page || got.PageSize != test.size || got.TotalItems != test.items {
				t.Fatalf("NewPagination() = %+v, want %d total pages", got, test.wantPages)
			}
		})
	}
}

func TestSuccess(t *testing.T) {
	recorder := serve(t, func(c *gin.Context) {
		Success(c, http.StatusCreated, gin.H{"id": "abc"})
	})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	var body envelopeBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if string(body.Data) != `{"id":"abc"}` {
		t.Fatalf("data = %s", body.Data)
	}
	if body.Meta.RequestID != "req-123" {
		t.Fatalf("requestId = %q, want req-123", body.Meta.RequestID)
	}
	parsed, err := time.Parse(time.RFC3339, body.Meta.Timestamp)
	if err != nil || parsed.Location() != time.UTC {
		t.Fatalf("timestamp %q is not an RFC 3339 UTC time (%v)", body.Meta.Timestamp, err)
	}
	if body.Meta.Pagination != nil {
		t.Fatal("a single-object response carries pagination")
	}
}

func TestSuccessList(t *testing.T) {
	recorder := serve(t, func(c *gin.Context) {
		SuccessList(c, http.StatusOK, []string{"a", "b"}, NewPagination(1, 20, 2))
	})

	var body envelopeBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if string(body.Data) != `["a","b"]` {
		t.Fatalf("data = %s", body.Data)
	}
	if body.Meta.Pagination == nil || body.Meta.Pagination.TotalPages != 1 || body.Meta.Pagination.TotalItems != 2 {
		t.Fatalf("pagination = %+v", body.Meta.Pagination)
	}
}
