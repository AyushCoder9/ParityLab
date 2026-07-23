package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/auth"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
)

func authenticatedHandler(t *testing.T, insecure bool) http.Handler {
	t.Helper()
	cipher, err := secrets.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("h", 32))))
	if err != nil {
		t.Fatal(err)
	}
	engineRepository := engine.NewMemoryRepository()
	engineService, err := engine.NewServiceWithRepository(engineRepository)
	if err != nil {
		t.Fatal(err)
	}
	authRepository := auth.NewMemoryRepository(engineRepository.ListStripeConnectionsForProject)
	return New(engineService, Config{
		WebOrigin: "http://127.0.0.1:3202", Auth: auth.NewService(authRepository, cipher),
		Stripe: engine.NewStripeService(engineRepository, httpStripeGateway{}, cipher), InsecureCookies: insecure,
	})
}

func registerCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(
		`{"email":"owner@example.test","password":"correct-horse-battery","workspace_name":"Workspace","project_name":"Project"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://127.0.0.1:3202")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", response.Code, response.Body.String())
	}
	var view auth.SessionView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || view.User.Email != "owner@example.test" {
		t.Fatalf("session=%#v err=%v", view, err)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%#v", cookies)
	}
	return cookies[0]
}

func TestAuthCookieCORSAndCSRFContract(t *testing.T) {
	handler := authenticatedHandler(t, false)
	cookie := registerCookie(t, handler)
	if cookie.Name != sessionCookieName || !cookie.HttpOnly || !cookie.Secure ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != int(auth.SessionTTL.Seconds()) {
		t.Fatalf("cookie=%#v", cookie)
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/v1/session", nil)
	sessionRequest.Header.Set("Origin", "http://127.0.0.1:3202")
	sessionRequest.AddCookie(cookie)
	sessionResponse := httptest.NewRecorder()
	handler.ServeHTTP(sessionResponse, sessionRequest)
	if sessionResponse.Code != http.StatusOK ||
		sessionResponse.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:3202" ||
		sessionResponse.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("session status=%d headers=%v body=%s", sessionResponse.Code, sessionResponse.Header(), sessionResponse.Body.String())
	}

	csrfRequest := httptest.NewRequest(http.MethodPatch, "/v1/settings/project", strings.NewReader(`{"name":"attacker"}`))
	csrfRequest.Header.Set("Content-Type", "application/json")
	csrfRequest.Header.Set("Origin", "https://attacker.invalid")
	csrfRequest.AddCookie(cookie)
	csrfResponse := httptest.NewRecorder()
	handler.ServeHTTP(csrfResponse, csrfRequest)
	if csrfResponse.Code != http.StatusForbidden || !strings.Contains(csrfResponse.Body.String(), `"csrf_origin_invalid"`) {
		t.Fatalf("csrf status=%d body=%s", csrfResponse.Code, csrfResponse.Body.String())
	}
}

func TestProtectedResourceAndLogout(t *testing.T) {
	handler := authenticatedHandler(t, true)
	anonymous := httptest.NewRecorder()
	handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/v1/settings/project", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d", anonymous.Code)
	}
	cookie := registerCookie(t, handler)
	if cookie.Secure {
		t.Fatal("loopback opt-in cookie unexpectedly secure")
	}
	logoutRequest := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	logoutRequest.Header.Set("Origin", "http://127.0.0.1:3202")
	logoutRequest.AddCookie(cookie)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logoutResponse.Code, logoutResponse.Body.String())
	}
	cleared := logoutResponse.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("clear cookie=%#v", cleared)
	}
	replay := httptest.NewRequest(http.MethodGet, "/v1/session", nil)
	replay.AddCookie(cookie)
	replayResponse := httptest.NewRecorder()
	handler.ServeHTTP(replayResponse, replay)
	if replayResponse.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status=%d", replayResponse.Code)
	}
}

func TestLoginLimiterPrunesExpiredAndBoundsUniqueKeys(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter(func() time.Time { return now })
	for index := 0; index < maxLoginBuckets+512; index++ {
		limiter.failure(formatRetryAfter(index + 1))
	}
	if len(limiter.buckets) > maxLoginBuckets {
		t.Fatalf("buckets grew without bound: %d", len(limiter.buckets))
	}
	now = now.Add(6 * time.Minute)
	for index := 0; index < 64; index++ {
		limiter.limited("sweep")
	}
	if len(limiter.buckets) != 0 {
		t.Fatalf("expired buckets not pruned: %d", len(limiter.buckets))
	}
}
