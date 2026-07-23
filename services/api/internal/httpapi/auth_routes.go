package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/auth"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
)

const sessionCookieName = "paritylab_session"

type loginBucket struct {
	failures int
	resetAt  time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]loginBucket
	now     func() time.Time
	ops     uint64
}

const maxLoginBuckets = 4096

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{buckets: make(map[string]loginBucket), now: now}
}

func (l *loginLimiter) limited(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.maintainLocked(now)
	bucket, ok := l.buckets[key]
	if !ok || !bucket.resetAt.After(now) {
		delete(l.buckets, key)
		return false, 0
	}
	if bucket.failures < 8 {
		return false, 0
	}
	return true, bucket.resetAt.Sub(now)
}

func (l *loginLimiter) failure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.maintainLocked(now)
	bucket := l.buckets[key]
	if !bucket.resetAt.After(now) {
		bucket = loginBucket{resetAt: now.Add(5 * time.Minute)}
	}
	bucket.failures++
	l.buckets[key] = bucket
}

func (l *loginLimiter) maintainLocked(now time.Time) {
	l.ops++
	if l.ops%64 != 0 && len(l.buckets) < maxLoginBuckets {
		return
	}
	for key, bucket := range l.buckets {
		if !bucket.resetAt.After(now) {
			delete(l.buckets, key)
		}
	}
	for len(l.buckets) >= maxLoginBuckets {
		for key := range l.buckets {
			delete(l.buckets, key)
			break
		}
	}
}

func (l *loginLimiter) success(key string) {
	l.mu.Lock()
	delete(l.buckets, key)
	l.mu.Unlock()
}

type registerRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	WorkspaceName string `json:"workspace_name"`
	ProjectName   string `json:"project_name"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	if !h.authAvailable(w, r) {
		return
	}
	var input registerRequest
	body, ok := h.decodeJSONBody(w, r, &input)
	if !ok {
		return
	}
	defer clear(body)
	view, token, apiErr := h.config.Auth.Register(r.Context(), input.Email, input.Password, input.WorkspaceName, input.ProjectName)
	input.Password = ""
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.setSessionCookie(w, token, auth.SessionTTL)
	h.writeJSON(w, http.StatusCreated, view)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.authAvailable(w, r) {
		return
	}
	var input loginRequest
	body, ok := h.decodeJSONBody(w, r, &input)
	if !ok {
		return
	}
	defer clear(body)
	key := clientAddress(r) + ":" + h.config.Auth.LoginThrottleKey(input.Email)
	if limited, retry := h.loginLimiter.limited(key); limited {
		seconds := int(retry.Round(time.Second).Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", formatRetryAfter(seconds))
		h.writeError(w, r, &domain.Error{Type: "rate_limit_error", Code: "rate_limit_exceeded", Message: "Too many sign-in attempts. Try again later.", HTTPStatus: http.StatusTooManyRequests})
		return
	}
	view, token, apiErr := h.config.Auth.Login(r.Context(), input.Email, input.Password)
	input.Password = ""
	if apiErr != nil {
		h.loginLimiter.failure(key)
		h.writeError(w, r, apiErr)
		return
	}
	h.loginLimiter.success(key)
	h.setSessionCookie(w, token, auth.SessionTTL)
	h.writeJSON(w, http.StatusOK, view)
}

func formatRetryAfter(seconds int) string {
	const digits = "0123456789"
	if seconds == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for seconds > 0 {
		index--
		buffer[index] = digits[seconds%10]
		seconds /= 10
	}
	return string(buffer[index:])
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if !h.authAvailable(w, r) {
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		if revokeErr := h.config.Auth.Logout(r.Context(), cookie.Value); revokeErr != nil {
			h.writeError(w, r, domain.Internal("logout_failed", "The session could not be revoked."))
			return
		}
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	h.writeJSON(w, http.StatusOK, identity)
}

func (h *Handler) authAvailable(w http.ResponseWriter, r *http.Request) bool {
	if h.config.Auth != nil {
		return true
	}
	h.writeError(w, r, &domain.Error{Type: "api_error", Code: "auth_not_configured", Message: "Authentication is not configured.", HTTPStatus: http.StatusServiceUnavailable})
	return false
}

func (h *Handler) requireSession(w http.ResponseWriter, r *http.Request) (auth.SessionView, bool) {
	if h.config.Auth == nil {
		return auth.SessionView{}, false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		h.writeError(w, r, domain.Unauthorized("session_required", "Authentication is required."))
		return auth.SessionView{}, false
	}
	identity, apiErr := h.config.Auth.Session(r.Context(), cookie.Value)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return auth.SessionView{}, false
	}
	return identity, true
}

func (h *Handler) optionalSession(w http.ResponseWriter, r *http.Request) (auth.SessionView, bool, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == http.ErrNoCookie {
		return auth.SessionView{}, false, true
	}
	if err != nil || cookie.Value == "" {
		h.writeError(w, r, domain.Unauthorized("session_invalid", "The session is invalid or expired."))
		return auth.SessionView{}, false, false
	}
	identity, apiErr := h.config.Auth.Session(r.Context(), cookie.Value)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return auth.SessionView{}, false, false
	}
	return identity, true, true
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: !h.config.InsecureCookies, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0),
		HttpOnly: true, Secure: !h.config.InsecureCookies, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) projectSettings(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	item, apiErr := h.config.Auth.Project(r.Context(), identity.Project.ID)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, item)
}

type updateProjectRequest struct {
	Name          string `json:"name"`
	RetentionDays int    `json:"retention_days"`
}

func (h *Handler) updateProjectSettings(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	var input updateProjectRequest
	if _, ok := h.decodeJSONBody(w, r, &input); !ok {
		return
	}
	item, apiErr := h.config.Auth.UpdateProject(r.Context(), identity, input.Name, input.RetentionDays)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, item)
}

func (h *Handler) environments(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	items, apiErr := h.config.Auth.Environments(r.Context(), identity.Project.ID)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, list(items))
}

func (h *Handler) selectEnvironment(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	item, apiErr := h.config.Auth.SelectEnvironment(r.Context(), identity, r.PathValue("id"))
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, item)
}

func (h *Handler) findings(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	items, apiErr := h.config.Auth.Findings(r.Context(), identity.Project.ID, r.URL.Query().Get("status"))
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, list(items))
}

func (h *Handler) resolveFinding(w http.ResponseWriter, r *http.Request) {
	h.setFindingResolution(w, r, true)
}

func (h *Handler) reopenFinding(w http.ResponseWriter, r *http.Request) {
	h.setFindingResolution(w, r, false)
}

func (h *Handler) setFindingResolution(w http.ResponseWriter, r *http.Request, resolved bool) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	item, apiErr := h.config.Auth.SetFindingResolved(r.Context(), identity, r.PathValue("id"), resolved)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, item)
}

func (h *Handler) notifications(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	items, apiErr := h.config.Auth.Notifications(r.Context(), identity.Project.ID)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, list(items))
}

func (h *Handler) readNotification(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	item, apiErr := h.config.Auth.MarkNotificationRead(r.Context(), identity, r.PathValue("id"))
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, item)
}

func (h *Handler) readAllNotifications(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	count, apiErr := h.config.Auth.MarkAllNotificationsRead(r.Context(), identity)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]int{"updated": count})
}

func (h *Handler) connections(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	items, apiErr := h.config.Auth.Connections(r.Context(), identity.Project.ID)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusOK, list(items))
}

func list[T any](items []T) map[string]any {
	return map[string]any{"object": "list", "data": items, "has_more": false}
}
