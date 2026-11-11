package servicehttp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

type photoHTTPStore struct {
	download courier.ProofPhotoDownload
	body     string
	err      error
	viewer   courier.PhotoViewer
}

func (s *photoHTTPStore) Photo(_ context.Context, _ string, viewer courier.PhotoViewer) (courier.ProofPhotoDownload, error) {
	s.viewer = viewer
	return s.download, s.err
}

func (s *photoHTTPStore) Open(context.Context, string, courier.PhotoViewer) (io.ReadCloser, string, int64, error) {
	if s.err != nil {
		return nil, "", 0, s.err
	}
	return io.NopCloser(strings.NewReader(s.body)), "image/jpeg", int64(len(s.body)), nil
}

func photoHTTPRequest(t *testing.T, store ProofPhotoStore, role auth.Role, target string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	secret := strings.Repeat("s", 32)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router, err := New(logger, Options{AccessSecret: secret, CourierProofPhoto: store})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := auth.NewTokenManager(secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue("11111111-1111-4111-8111-111111111111", role)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestProofPhotoRouteReturnsPresignedURL(t *testing.T) {
	store := &photoHTTPStore{download: courier.ProofPhotoDownload{
		URL: "https://storage.example/proof?signature=short", ExpiresAt: time.Now().Add(time.Minute), MIMEType: "image/jpeg",
	}}
	recorder := photoHTTPRequest(t, store, auth.RoleCustomer, "/parcels/11111111-1111-4111-8111-111111111111/proof/photo")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "https://storage.example/proof") || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("body = %s headers = %v", recorder.Body.String(), recorder.Header())
	}
	if store.viewer.UserID == "" || store.viewer.Staff || store.viewer.Courier {
		t.Fatalf("viewer = %+v", store.viewer)
	}
}

func TestProofPhotoRouteMapsAuthorizationErrors(t *testing.T) {
	for err, want := range map[error]int{
		courier.ErrForbidden:          http.StatusForbidden,
		courier.ErrNotFound:           http.StatusNotFound,
		courier.ErrProofPhotoNotFound: http.StatusNotFound,
		courier.ErrUnavailable:        http.StatusServiceUnavailable,
	} {
		recorder := photoHTTPRequest(t, &photoHTTPStore{err: err}, auth.RoleCourier,
			"/parcels/11111111-1111-4111-8111-111111111111/proof/photo")
		if recorder.Code != want {
			t.Fatalf("%v: status = %d body = %s", err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestProofPhotoRouteRejectsInvalidUUID(t *testing.T) {
	recorder := photoHTTPRequest(t, &photoHTTPStore{}, auth.RoleCustomer, "/parcels/not-a-uuid/proof/photo")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestProofPhotoContentStreams(t *testing.T) {
	store := &photoHTTPStore{body: "private photo"}
	recorder := photoHTTPRequest(t, store, auth.RoleCustomer, "/parcels/11111111-1111-4111-8111-111111111111/proof/photo/content")
	if recorder.Code != http.StatusOK || recorder.Body.String() != "private photo" {
		t.Fatalf("status = %d body = %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "image/jpeg" || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", recorder.Header())
	}
}
