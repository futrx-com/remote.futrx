package httphandlers

import (
	"errors"
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// authTwoFactorHandler completes the pending-login-to-real-session step for
// accounts with 2FA enabled. Its routes live under /auth/2fa/* (pre-session,
// like /auth/local/login), reachable before a real session cookie exists.
type authTwoFactorHandler struct {
	auth    *serviceauth.Service
	limiter *localLoginLimiter
}

func (h *authTwoFactorHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/2fa/verify", h.verify)
	mux.HandleFunc("/auth/2fa/cancel", h.cancel)
}

func (h *authTwoFactorHandler) verify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := readJSONBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	key := localLoginRateLimitKey(r)
	if !h.limiter.Allow(key) {
		w.Header().Set("Retry-After", "300")
		httptransport.SendErr(w, http.StatusTooManyRequests, "too many attempts; try again in a few minutes")
		return
	}

	pendingToken := pendingCookieValue(r)
	if pendingToken == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "no pending two-factor challenge")
		return
	}

	result, err := h.auth.CompleteTwoFactorChallenge(r.Context(), pendingToken, body.Code, localClientIP(r), r.UserAgent())
	if err != nil {
		h.limiter.Failure(key)
		status := http.StatusUnauthorized
		if errors.Is(err, serviceauth.ErrInvalidPendingLogin) {
			status = http.StatusBadRequest
		}
		httptransport.SendErr(w, status, "invalid or expired code")
		return
	}
	h.limiter.Success(key)

	clearPendingCookie(w, h.auth)
	setSessionCookie(w, h.auth, result.CookieValue)
	httptransport.SendJSON(w, http.StatusOK, h.auth.Status(r.Context(), result.CookieValue))
}

func (h *authTwoFactorHandler) cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	clearPendingCookie(w, h.auth)
	w.WriteHeader(http.StatusNoContent)
}
