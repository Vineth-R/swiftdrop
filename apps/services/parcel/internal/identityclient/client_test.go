package identityclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCheckEligibility(t *testing.T) {
	tests := []struct {
		name       string
		courierID  string
		statusCode int
		body       string
		wantErr    error
	}{
		{
			name:       "eligible courier",
			courierID:  "c1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f",
			statusCode: http.StatusOK,
			body:       `{"data":{"isActive":true,"isAvailable":true,"role":"COURIER"}}`,
			wantErr:    nil,
		},
		{
			name:       "inactive courier",
			courierID:  "c1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f",
			statusCode: http.StatusOK,
			body:       `{"data":{"isActive":false,"isAvailable":true,"role":"COURIER"}}`,
			wantErr:    ErrCourierNotEligible,
		},
		{
			name:       "unavailable courier",
			courierID:  "c1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f",
			statusCode: http.StatusOK,
			body:       `{"data":{"isActive":true,"isAvailable":false,"role":"COURIER"}}`,
			wantErr:    ErrCourierNotEligible,
		},
		{
			name:       "wrong role",
			courierID:  "c1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f",
			statusCode: http.StatusOK,
			body:       `{"data":{"isActive":true,"isAvailable":true,"role":"DISPATCHER"}}`,
			wantErr:    ErrCourierNotEligible,
		},
		{
			name:       "courier not found",
			courierID:  "c1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f",
			statusCode: http.StatusNotFound,
			body:       `{"error":{"code":"NOT_FOUND","message":"courier not found"}}`,
			wantErr:    ErrCourierNotEligible,
		},
		{
			name:      "empty courier ID",
			courierID: "",
			wantErr:   ErrCourierNotEligible,
		},
		{
			name:       "identity unauthorized",
			courierID:  "c1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":{"code":"UNAUTHORIZED","message":"bad token"}}`,
			wantErr:    ErrServiceUnavailable,
		},
		{
			name:       "identity server error",
			courierID:  "c1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f",
			statusCode: http.StatusInternalServerError,
			body:       `{"error":{"code":"INTERNAL_ERROR","message":"db error"}}`,
			wantErr:    ErrServiceUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Internal-Token") != "test-token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if r.Header.Get("X-Request-Id") == "" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			})

			client, err := New("http://identity.internal", "test-token", time.Second)
			if err != nil {
				t.Fatalf("New() unexpected error: %v", err)
			}
			client.WithTransport(roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, r)
				return rec.Result(), nil
			}))

			err = client.CheckEligibility(context.Background(), tc.courierID)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestListCouriers(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/couriers" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "c1", "fullName": "Kasun Silva", "isAvailable": true},
				{"id": "c2", "fullName": "Amal Fernando", "isAvailable": false}
			]
		}`))
	})

	client, err := New("http://identity.internal", "test-token", time.Second)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	client.WithTransport(roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec.Result(), nil
	}))

	couriers, err := client.ListCouriers(context.Background(), "Bearer fake-token", nil)
	if err != nil {
		t.Fatalf("ListCouriers() error: %v", err)
	}
	if len(couriers) != 2 {
		t.Fatalf("expected 2 couriers, got %d", len(couriers))
	}
	if couriers[0].ID != "c1" || couriers[0].FullName != "Kasun Silva" || !couriers[0].IsAvailable {
		t.Fatalf("unexpected courier 0: %+v", couriers[0])
	}
}

func TestNewValidatesURL(t *testing.T) {
	badURLs := []string{
		"",
		"ftp://example.com",
		"http://",
		"invalid-url",
	}

	for _, u := range badURLs {
		if _, err := New(u, "token", time.Second); err == nil {
			t.Errorf("expected error for URL %q", u)
		}
	}
}
