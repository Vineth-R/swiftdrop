package identityclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

func TestSessionStateAndContact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "internal-token" || r.Header.Get("X-Request-Id") == "" {
			t.Errorf("missing internal headers")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/users/user-1/session-state":
			_, _ = io.WriteString(w, `{"data":{"role":"MERCHANT","isActive":true,"sessionVersion":3}}`)
		case "/internal/users/user-1/contact":
			_, _ = io.WriteString(w, `{"data":{"fullName":"Owner","email":"owner@example.test","phone":"+94771234567"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "internal-token", time.Second)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	state, err := client.SessionState(context.Background(), "user-1")
	if err != nil || state.Role != "MERCHANT" || state.SessionVersion != 3 || !state.IsActive {
		t.Fatalf("SessionState() = %+v, %v", state, err)
	}
	contact, err := client.Contact(context.Background(), "user-1")
	if err != nil || contact.FullName != "Owner" || contact.Email != "owner@example.test" {
		t.Fatalf("Contact() = %+v, %v", contact, err)
	}
}

func TestOwnerAndLifecycleRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/merchant-owners":
			_, _ = io.WriteString(w, `{"data":{"userId":"user-1","created":true}}`)
		case "/internal/merchant-owners/user-1/lifecycle":
			_, _ = io.WriteString(w, `{"data":{"userId":"user-1","transitionId":"t-1","sequence":2,"operation":"APPROVE","alreadyApplied":false,"sessionVersion":2,"isActive":true,"revokedSessions":0}}`)
		case "/internal/merchant-owners/user-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, _ := New(server.URL, "", time.Second)

	owner, err := client.CreateMerchantOwner(context.Background(), OwnerInput{Email: "owner@example.test", Password: "secret", FullName: "Owner", Phone: "+94771234567"})
	if err != nil || owner.UserID != "user-1" || !owner.Created {
		t.Fatalf("CreateMerchantOwner() = %+v, %v", owner, err)
	}
	lifecycle, err := client.ApplyLifecycle(context.Background(), "user-1", LifecycleInput{TransitionID: "t-1", Sequence: 2, Operation: "APPROVE"})
	if err != nil || lifecycle.SessionVersion != 2 || !lifecycle.IsActive {
		t.Fatalf("ApplyLifecycle() = %+v, %v", lifecycle, err)
	}
	if err := client.DeleteMerchantOwner(context.Background(), "user-1"); err != nil {
		t.Fatalf("DeleteMerchantOwner() error = %v", err)
	}
}

func TestClientMapsUnavailableAndNotFound(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		want   error
	}{
		{"unavailable", http.StatusServiceUnavailable, ErrUnavailable},
		{"not found", http.StatusNotFound, ErrNotFound},
		{"conflict", http.StatusConflict, ErrTransitionConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, `{"error":{"code":"X"}}`)
			}))
			defer server.Close()
			client, _ := New(server.URL, "", time.Second)
			if _, err := client.SessionState(context.Background(), "user-1"); !errors.Is(err, test.want) {
				t.Fatalf("SessionState() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	if _, err := New("identity:8081", "", time.Second); err == nil {
		t.Fatal("New() accepted a relative URL")
	}
}

func TestCreateMerchantOwnerPropagatesInboundRequestID(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(requestid.Header)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"userId":"user-1","created":true}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	ctx := requestid.WithContext(context.Background(), "inbound-request-id")
	if _, err := client.CreateMerchantOwner(ctx, OwnerInput{Email: "owner@example.com"}); err != nil {
		t.Fatal(err)
	}
	if got != "inbound-request-id" {
		t.Fatalf("X-Request-Id = %q, want the inbound request ID", got)
	}
}

func TestCreateMerchantOwnerGeneratesRequestIDWithoutInbound(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(requestid.Header)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"userId":"user-1","created":true}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateMerchantOwner(context.Background(), OwnerInput{Email: "owner@example.com"}); err != nil {
		t.Fatal(err)
	}
	if !requestid.Valid(got) {
		t.Fatalf("X-Request-Id = %q, want a generated valid ID", got)
	}
}
