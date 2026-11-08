package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newDesktopAuthTestRouter(t *testing.T) (*DesktopService, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := &DesktopService{apiToken: "desktop-test-token"}
	router := gin.New()
	router.Use(service.attachDesktopBootstrapCookie())
	service.registerDesktopRoutes(router)
	return service, router
}

func desktopAPITokenCookieFrom(recorder *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == desktopAPITokenCookieName {
			return cookie
		}
	}
	return nil
}

// An unauthenticated client must never receive the desktop credential from an
// ordinary endpoint response.
func TestDesktopMetaResponseDoesNotLeakDesktopCredential(t *testing.T) {
	_, router := newDesktopAuthTestRouter(t)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/desktop/meta", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("unauthenticated meta response set cookies: %v", cookies)
	}
	if strings.Contains(recorder.Body.String(), "desktop-test-token") {
		t.Fatalf("meta response leaked the desktop api token")
	}
}

// The native host injects a single-use nonce; the server trades it for a
// session cookie at most once.
func TestDesktopBootstrapNonceIssuesCookieOnce(t *testing.T) {
	service, router := newDesktopAuthTestRouter(t)

	nonce, err := service.newBootstrapNonce()
	if err != nil {
		t.Fatalf("newBootstrapNonce: %v", err)
	}

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/?desktop_bootstrap="+nonce, nil))
	cookie := desktopAPITokenCookieFrom(first)
	if cookie == nil {
		t.Fatalf("bootstrap response did not issue a desktop cookie")
	}
	if cookie.Value != service.apiToken {
		t.Fatalf("cookie value = %q, want the desktop token", cookie.Value)
	}
	if cookie.MaxAge != 0 {
		t.Fatalf("bootstrap cookie MaxAge = %d, want a session cookie", cookie.MaxAge)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/?desktop_bootstrap="+nonce, nil))
	if replay := desktopAPITokenCookieFrom(second); replay != nil {
		t.Fatalf("replayed bootstrap nonce issued another desktop cookie")
	}
}

// The bootstrap cookie is a transport for the webview, not an authentication
// factor: desktop routes still require the explicit request header.
func TestDesktopAPITokenRequiresHeaderNotCookie(t *testing.T) {
	service, router := newDesktopAuthTestRouter(t)

	cookieOnly := httptest.NewRequest(http.MethodGet, "/api/v1/desktop/window/state", nil)
	cookieOnly.AddCookie(&http.Cookie{Name: desktopAPITokenCookieName, Value: service.apiToken})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, cookieOnly)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cookie-only status = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	withHeader := httptest.NewRequest(http.MethodGet, "/api/v1/desktop/window/state", nil)
	withHeader.Header.Set("X-Hephaestus-Desktop-Token", service.apiToken)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, withHeader)
	if recorder.Code != http.StatusOK {
		t.Fatalf("header status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestConsumeDesktopBootstrapNonceExpiresAndIsSingleUse(t *testing.T) {
	service := &DesktopService{
		apiToken: "desktop-test-token",
		bootstrapNonces: map[string]time.Time{
			"expired": time.Now().Add(-time.Second),
			"fresh":   time.Now().Add(time.Minute),
		},
	}

	if service.consumeBootstrapNonce("expired") {
		t.Fatalf("expired nonce was accepted")
	}
	if !service.consumeBootstrapNonce("fresh") {
		t.Fatalf("fresh nonce was rejected")
	}
	if service.consumeBootstrapNonce("fresh") {
		t.Fatalf("nonce reuse was accepted")
	}
	if service.consumeBootstrapNonce("   ") {
		t.Fatalf("blank nonce was accepted")
	}
}
