package httphandlers

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type localAuthHandler struct {
	auth   *serviceauth.Service
	logins *localLoginLimiter
}

func (h *localAuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/local/claim", h.claim)
	mux.HandleFunc("/auth/local/login", h.login)
}

func (h *localAuthHandler) claim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSONBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	key := localClientIP(r) + "|claim"
	if !h.logins.Allow(key) {
		w.Header().Set("Retry-After", "300")
		httptransport.SendErr(w, http.StatusTooManyRequests, "too many attempts; try again in a few minutes")
		return
	}
	authorizedEmail, _ := callerEmailFromRequest(r, h.auth)
	user, err := h.auth.ClaimLocalAdmin(r.Context(), body.Email, body.Password, authorizedEmail)
	if err != nil {
		h.logins.Failure(key)
		switch {
		case errors.Is(err, serviceauth.ErrLocalAdminAlreadyClaimed):
			httptransport.SendErr(w, http.StatusConflict, err.Error())
		case errors.Is(err, serviceauth.ErrAdminClaimUnauthorized):
			httptransport.SendErr(w, http.StatusForbidden, err.Error())
		case errors.Is(err, serviceauth.ErrPasswordTooShort),
			errors.Is(err, serviceauth.ErrPasswordTooLong):
			httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		default:
			httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	h.logins.Success(key)
	// A brand-new account cannot have a 2FA record yet, so claim always
	// issues a session directly rather than going through CompletePasswordLogin.
	cookieValue, err := h.auth.IssueSession(r.Context(), user, serviceauth.SignInMethodPassword, localClientIP(r), r.UserAgent())
	if err != nil {
		httptransport.SendErr(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	setSessionCookie(w, h.auth, cookieValue)
	httptransport.SendJSON(w, http.StatusCreated, h.auth.Status(r.Context(), cookieValue))
}

func (h *localAuthHandler) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSONBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	key := localLoginRateLimitKey(r)
	if !h.logins.Allow(key) {
		w.Header().Set("Retry-After", "300")
		httptransport.SendErr(w, http.StatusTooManyRequests, "too many attempts; try again in a few minutes")
		return
	}
	result, err := h.auth.CompletePasswordLogin(r.Context(), body.Email, body.Password, localClientIP(r), r.UserAgent())
	if err != nil {
		h.logins.Failure(key)
		httptransport.SendErr(w, http.StatusUnauthorized, serviceauth.ErrInvalidCredentials.Error())
		return
	}
	h.logins.Success(key)
	if !result.Completed {
		setPendingCookie(w, h.auth, result.PendingToken)
		httptransport.SendJSON(w, http.StatusOK, map[string]bool{"twoFactorRequired": true})
		return
	}
	setSessionCookie(w, h.auth, result.CookieValue)
	httptransport.SendJSON(w, http.StatusOK, h.auth.Status(r.Context(), result.CookieValue))
}

const localLoginWindow = 5 * time.Minute

type localLoginAttempt struct {
	Failures int
	ResetAt  time.Time
}

type localLoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]localLoginAttempt
}

func newLocalLoginLimiter() *localLoginLimiter {
	return &localLoginLimiter{attempts: make(map[string]localLoginAttempt)}
}

func (l *localLoginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, ok := l.attempts[key]
	if !ok || time.Now().After(attempt.ResetAt) {
		delete(l.attempts, key)
		return true
	}
	return attempt.Failures < 5
}

func (l *localLoginLimiter) Failure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	attempt := l.attempts[key]
	if attempt.ResetAt.Before(now) {
		attempt = localLoginAttempt{ResetAt: now.Add(localLoginWindow)}
	}
	attempt.Failures++
	l.attempts[key] = attempt
}

func (l *localLoginLimiter) Success(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func localClientIP(r *http.Request) string {
	ip := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	return ip
}

func localLoginRateLimitKey(r *http.Request) string {
	return localClientIP(r) + "|login"
}
