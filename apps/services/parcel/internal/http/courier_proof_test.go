package servicehttp

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	proofmedia "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/proof"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

type courierProofFixture struct {
	input courier.ProofInput
	err   error
	calls int
}

func (f *courierProofFixture) CreateProof(_ context.Context, input courier.ProofInput) (courier.ProofResult, error) {
	f.input, f.calls = input, f.calls+1
	return courier.ProofResult{ID: "00000000-0000-4000-8000-000000000073", ParcelID: input.ParcelID,
		RecipientName: stringPointer(input.RecipientName), Note: stringPointer(input.Note), HasPhoto: input.Photo != nil,
		CreatedAt: time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)}, f.err
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func proofRouter(t *testing.T, store CourierProofStore) *gin.Engine {
	t.Helper()
	return reportRouter(t, Options{AccessSecret: reportTestSecret, CourierProof: store})
}

func proofRequest(t *testing.T, router *gin.Engine, role auth.Role, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	return proofRequestPath(t, router, role, "/courier/deliveries/"+courierParcelID+"/proof", body, contentType)
}

func proofRequestPath(t *testing.T, router *gin.Engine, role auth.Role, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if role != "" {
		manager, err := auth.NewTokenManager(reportTestSecret, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := manager.Issue(courierTestID, role)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: "sd_access", Value: token})
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func proofForm(t *testing.T, fields map[string]string, photo []byte, photoType string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if photo != nil {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="photo"; filename="client-name.jpg"`)
		header.Set("Content-Type", photoType)
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(photo); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func TestCourierProofAuthorizationAndSuccess(t *testing.T) {
	body, contentType := proofForm(t, map[string]string{"recipientName": " Amali ", "note": "At door"}, nil, "")
	for _, tc := range []struct {
		name string
		role auth.Role
		want int
	}{
		{name: "signed out", want: 401},
		{name: "customer", role: auth.RoleCustomer, want: 403},
		{name: "dispatcher", role: auth.RoleDispatcher, want: 403},
		{name: "admin", role: auth.RoleAdmin, want: 403},
		{name: "courier", role: auth.RoleCourier, want: 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &courierProofFixture{}
			recorder := proofRequest(t, proofRouter(t, store), tc.role, body, contentType)
			if recorder.Code != tc.want || recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("response %d %s", recorder.Code, recorder.Body)
			}
			if tc.want != 201 && store.calls != 0 {
				t.Fatal("unauthorized request reached proof store")
			}
			if tc.want == 201 && (store.input.CourierID != courierTestID || store.input.ParcelID != courierParcelID || strings.Contains(recorder.Body.String(), "photo_path")) {
				t.Fatalf("unsafe or incorrect result: input=%+v body=%s", store.input, recorder.Body)
			}
		})
	}
}

func TestCourierProofFormErrors(t *testing.T) {
	valid, _ := proofForm(t, map[string]string{"note": "At door"}, nil, "")
	oversize, oversizeType := proofForm(t, nil, bytes.Repeat([]byte("x"), proofmedia.MaxPhotoBytes+1), "image/jpeg")
	for _, tc := range []struct {
		name, contentType string
		body              []byte
		want              int
		code              string
	}{
		{name: "wrong content type", contentType: "application/json", body: valid, want: 415, code: "UNSUPPORTED_MEDIA_TYPE"},
		{name: "malformed multipart", contentType: "multipart/form-data", body: []byte("bad"), want: 422, code: "VALIDATION_ERROR"},
		{name: "request too large", contentType: oversizeType, body: oversize, want: 413, code: "PAYLOAD_TOO_LARGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &courierProofFixture{}
			recorder := proofRequest(t, proofRouter(t, store), auth.RoleCourier, tc.body, tc.contentType)
			if recorder.Code != tc.want || !strings.Contains(recorder.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("response %d %s", recorder.Code, recorder.Body)
			}
			if store.calls != 0 {
				t.Fatal("invalid form reached proof store")
			}
		})
	}
}

func TestCourierProofStoreErrors(t *testing.T) {
	body, contentType := proofForm(t, map[string]string{"note": "At door"}, nil, "")
	for _, tc := range []struct {
		name, code string
		err        error
		want       int
	}{
		{name: "missing", err: courier.ErrNotFound, want: 404, code: "PARCEL_NOT_FOUND"},
		{name: "foreign", err: courier.ErrForbidden, want: 403, code: "FORBIDDEN"},
		{name: "empty", err: courier.ErrProofEmpty, want: 422, code: "VALIDATION_ERROR"},
		{name: "oversize", err: proofmedia.ErrPhotoTooLarge, want: 413, code: "PAYLOAD_TOO_LARGE"},
		{name: "unsupported", err: courier.ErrProofUnsupported, want: 415, code: "UNSUPPORTED_MEDIA_TYPE"},
		{name: "unavailable", err: courier.ErrUnavailable, want: 503, code: "SERVICE_UNAVAILABLE"},
		{name: "limit", err: courier.ErrProofLimitReached, want: 409, code: "PROOF_LIMIT_REACHED"},
		{name: "inconsistent assignment", err: courier.ErrInconsistentAssignment, want: 500, code: "INTERNAL_ERROR"},
		{name: "database", err: errors.New("secret SQL path"), want: 500, code: "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &courierProofFixture{err: tc.err}
			recorder := proofRequest(t, proofRouter(t, store), auth.RoleCourier, body, contentType)
			if recorder.Code != tc.want || !strings.Contains(recorder.Body.String(), `"code":"`+tc.code+`"`) || strings.Contains(recorder.Body.String(), "secret SQL") {
				t.Fatalf("response %d %s", recorder.Code, recorder.Body)
			}
		})
	}
}

type proofMultipartPart struct {
	name, filename, contentType, value string
}

func rawProofForm(t *testing.T, parts []proofMultipartPart) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		header := make(textproto.MIMEHeader)
		disposition := `form-data; name="` + part.name + `"`
		if part.filename != "" {
			disposition += `; filename="` + part.filename + `"`
		}
		header.Set("Content-Disposition", disposition)
		if part.contentType != "" {
			header.Set("Content-Type", part.contentType)
		}
		section, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := section.Write([]byte(part.value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func TestCourierProofInvalidParcelID(t *testing.T) {
	body, contentType := proofForm(t, map[string]string{"note": "At door"}, nil, "")
	store := &courierProofFixture{}
	recorder := proofRequestPath(t, proofRouter(t, store), auth.RoleCourier,
		"/courier/deliveries/not-a-uuid/proof", body, contentType)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response %d %s", recorder.Code, recorder.Body)
	}
	if store.calls != 0 {
		t.Fatal("invalid parcel reached proof store")
	}
}

func TestCourierProofRejectsDuplicateAndUnknownFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parts []proofMultipartPart
	}{
		{name: "duplicate note", parts: []proofMultipartPart{{name: "note", value: "one"}, {name: "note", value: "two"}}},
		{name: "unknown field", parts: []proofMultipartPart{{name: "recipient", value: "Amali"}}},
		{name: "text field with filename", parts: []proofMultipartPart{{name: "note", filename: "note.txt", value: "at door"}}},
		{name: "photo without filename", parts: []proofMultipartPart{{name: "photo", contentType: "image/png", value: "x"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, contentType := rawProofForm(t, tc.parts)
			store := &courierProofFixture{}
			recorder := proofRequest(t, proofRouter(t, store), auth.RoleCourier, body, contentType)
			if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), `"code":"VALIDATION_ERROR"`) {
				t.Fatalf("response %d %s", recorder.Code, recorder.Body)
			}
			if store.calls != 0 {
				t.Fatal("invalid multipart reached proof store")
			}
		})
	}
}

func TestCourierProofExactPhotoLimitIsAccepted(t *testing.T) {
	body, contentType := rawProofForm(t, []proofMultipartPart{{
		name: "photo", filename: "large.png", contentType: "image/png", value: string(bytes.Repeat([]byte("x"), proofmedia.MaxPhotoBytes)),
	}})
	store := &courierProofFixture{}
	recorder := proofRequest(t, proofRouter(t, store), auth.RoleCourier, body, contentType)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("response %d %s", recorder.Code, recorder.Body)
	}
	photoLength := 0
	if store.input.Photo != nil {
		photoLength = len(store.input.Photo.Data)
	}
	if store.calls != 1 || photoLength != proofmedia.MaxPhotoBytes {
		t.Fatalf("store calls=%d photo length=%d", store.calls, photoLength)
	}
}

func TestCourierProofRateLimitedPerCourier(t *testing.T) {
	body, contentType := proofForm(t, map[string]string{"note": "At door"}, nil, "")
	store := &courierProofFixture{}
	router := proofRouter(t, store)
	for index := 0; index < proofUploadsPerMinute; index++ {
		recorder := proofRequest(t, router, auth.RoleCourier, body, contentType)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("request %d response %d %s", index+1, recorder.Code, recorder.Body)
		}
	}
	recorder := proofRequest(t, router, auth.RoleCourier, body, contentType)
	if recorder.Code != http.StatusTooManyRequests || !strings.Contains(recorder.Body.String(), `"code":"RATE_LIMITED"`) {
		t.Fatalf("limited response %d %s", recorder.Code, recorder.Body)
	}
	if recorder.Header().Get("Retry-After") == "" || recorder.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("rate limit headers = %v", recorder.Header())
	}
	if store.calls != proofUploadsPerMinute {
		t.Fatalf("store calls = %d", store.calls)
	}
}
