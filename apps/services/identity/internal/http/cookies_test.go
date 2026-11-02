package servicehttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
)

var testCookies = CookieConfig{
	AccessName:  "sd_access",
	RefreshName: "sd_refresh",
	Secure:      true,
	SameSite:    http.SameSiteStrictMode,
}

func cookieByName(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response did not set the %s cookie", name)
	return nil
}

func TestCookieConfig_SetSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	testCookies.SetSession(context, service.Session{
		AccessToken:      "access-value",
		AccessExpiresAt:  time.Now().Add(15 * time.Minute),
		RefreshToken:     "rt_refresh-value",
		RefreshExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	})

	access := cookieByName(t, recorder, "sd_access")
	refresh := cookieByName(t, recorder, "sd_refresh")

	for _, cookie := range []*http.Cookie{access, refresh} {
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
			t.Fatalf("%s flags: HttpOnly=%t Secure=%t SameSite=%v", cookie.Name, cookie.HttpOnly, cookie.Secure, cookie.SameSite)
		}
	}
	if access.Value != "access-value" || access.Path != "/" || access.MaxAge < 890 || access.MaxAge > 900 {
		t.Fatalf("access cookie = %+v", access)
	}
	if refresh.Value != "rt_refresh-value" || refresh.Path != refreshCookiePath {
		t.Fatalf("refresh cookie = %+v", refresh)
	}
	if want := 7 * 24 * 3600; refresh.MaxAge < want-10 || refresh.MaxAge > want {
		t.Fatalf("refresh Max-Age = %d, want about %d", refresh.MaxAge, want)
	}
}

func TestCookieConfig_Clear(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	testCookies.Clear(context)

	access := cookieByName(t, recorder, "sd_access")
	refresh := cookieByName(t, recorder, "sd_refresh")
	if access.MaxAge >= 0 || access.Value != "" || access.Path != "/" {
		t.Fatalf("access cookie was not cleared: %+v", access)
	}
	// The path must match the one used when the cookie was set.
	if refresh.MaxAge >= 0 || refresh.Value != "" || refresh.Path != refreshCookiePath {
		t.Fatalf("refresh cookie was not cleared: %+v", refresh)
	}
}

func TestCookieConfig_RefreshToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("reads the refresh cookie", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		request.AddCookie(&http.Cookie{Name: "sd_refresh", Value: "rt_value"})
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = request

		if got := testCookies.RefreshToken(context); got != "rt_value" {
			t.Fatalf("RefreshToken() = %q, want rt_value", got)
		}
	})

	t.Run("is empty without the cookie", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)

		if got := testCookies.RefreshToken(context); got != "" {
			t.Fatalf("RefreshToken() = %q, want empty", got)
		}
	})
}

func TestSeconds(t *testing.T) {
	tests := []struct {
		name     string
		lifetime time.Duration
		want     int
	}{
		{name: "positive", lifetime: 90 * time.Second, want: 90},
		{name: "sub-second rounds down", lifetime: 1500 * time.Millisecond, want: 1},
		{name: "zero deletes", lifetime: 0, want: -1},
		{name: "past deletes", lifetime: -time.Minute, want: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := seconds(test.lifetime); got != test.want {
				t.Fatalf("seconds() = %d, want %d", got, test.want)
			}
		})
	}
}
