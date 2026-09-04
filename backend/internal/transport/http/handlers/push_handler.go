package httphandlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicepresence "github.com/futrx-com/remote.futrx.com/internal/service/presence"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// PushHandler exposes the browser side of Web Push: the application server key
// to subscribe with, this user's device registrations, and the heartbeat that
// says which chat they are watching.
type PushHandler struct {
	push     *servicepush.Service
	auth     *serviceauth.Service
	presence *servicepresence.Service
}

func NewPushHandler(
	push *servicepush.Service,
	auth *serviceauth.Service,
	presence *servicepresence.Service,
) *PushHandler {
	return &PushHandler{push: push, auth: auth, presence: presence}
}

func (h *PushHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/push/config", h.HandleConfig)
	mux.HandleFunc("/api/push/subscriptions", h.HandleSubscriptions)
	mux.HandleFunc("/api/push/subscriptions/status", h.HandleSubscriptionOwnership)
	mux.HandleFunc("/api/push/test", h.HandleTest)
	mux.HandleFunc("/api/push/presence", h.HandlePresence)
}

type pushConfigResponse struct {
	Enabled    bool   `json:"enabled"`
	PublicKey  string `json:"publicKey,omitempty"`
	Subscribed bool   `json:"subscribed"`
}

// subscriptionRequest mirrors the shape of PushSubscription.toJSON() so the
// browser can post its subscription unmodified.
type subscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type subscriptionEndpointRequest struct {
	Endpoint string `json:"endpoint"`
}

type subscriptionOwnershipResponse struct {
	Owned bool `json:"owned"`
}

func (h *PushHandler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	email, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !h.push.Enabled() {
		httptransport.SendJSON(w, http.StatusOK, pushConfigResponse{})
		return
	}

	subscribed, err := h.push.HasSubscriptions(r.Context(), email)
	if err != nil {
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, pushConfigResponse{
		Enabled:    true,
		PublicKey:  h.push.PublicKey(),
		Subscribed: subscribed,
	})
}

func (h *PushHandler) HandleSubscriptions(w http.ResponseWriter, r *http.Request) {
	email, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var body subscriptionRequest
		if err := decodePushBody(r, &body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		err := h.push.Subscribe(r.Context(), email, servicepush.Subscription{
			Endpoint:  body.Endpoint,
			P256dh:    body.Keys.P256dh,
			Auth:      body.Keys.Auth,
			UserAgent: r.UserAgent(),
		})
		if err != nil {
			sendPushError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		var body subscriptionEndpointRequest
		if err := decodePushBody(r, &body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := h.push.Unsubscribe(r.Context(), email, body.Endpoint); err != nil {
			sendPushError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleSubscriptionOwnership checks whether an origin-wide browser
// subscription belongs to the authenticated account. A local PushSubscription
// may have been created by a different user who previously signed in from the
// same browser profile.
func (h *PushHandler) HandleSubscriptionOwnership(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	email, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body subscriptionEndpointRequest
	if err := decodePushBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	owned, err := h.push.OwnsSubscription(r.Context(), email, body.Endpoint)
	if err != nil {
		sendPushError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, subscriptionOwnershipResponse{Owned: owned})
}

// HandleTest delivers one notification to the caller's own devices, which is
// the only way to tell a broken subscription from a quiet agent.
func (h *PushHandler) HandleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	email, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !h.push.Enabled() {
		sendPushError(w, servicepush.ErrDisabled)
		return
	}

	h.push.Notify(r.Context(), []string{email}, servicepush.Notification{
		Kind:  servicepush.KindTest,
		Title: "Notifications are working",
		Body:  "You will be notified when an agent asks a question or finishes a turn.",
		Tag:   "push-test",
	})
	w.WriteHeader(http.StatusNoContent)
}

// presenceRequest is one client saying what it currently has on screen. The
// client id distinguishes tabs, so a background tab signing off cannot cancel
// the claim of the focused one beside it.
type presenceRequest struct {
	ChatID   string `json:"chatId"`
	ClientID string `json:"clientId"`
	Revision uint64 `json:"revision"`
}

// HandlePresence records that the caller is watching a chat right now, which
// keeps notifications about it away from every device they own. An empty chat
// id withdraws the claim; claims also expire on their own, so a browser that
// vanishes without one starts notifying again shortly.
func (h *PushHandler) HandlePresence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	email, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body presenceRequest
	if err := decodePushBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Revision == 0 {
		httptransport.SendErr(w, http.StatusBadRequest, "presence revision is required")
		return
	}

	h.presence.Record(email, servicepresence.Report{
		ClientID: body.ClientID,
		ChatID:   body.ChatID,
		Revision: body.Revision,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *PushHandler) caller(r *http.Request) (string, error) {
	if h.auth == nil {
		return "local-admin", nil
	}
	session, err := httptransport.NewPrincipalResolver(h.auth).Session(r)
	if err != nil {
		return "", err
	}
	if session == nil {
		return "", errors.New("no session")
	}
	return session.Email, nil
}

func decodePushBody(r *http.Request, target any) error {
	err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(target)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func sendPushError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, servicepush.ErrDisabled):
		httptransport.SendErr(w, http.StatusServiceUnavailable, "push notifications are not configured")
	case errors.Is(err, servicepush.ErrInvalidIdentity):
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
	case errors.Is(err, servicepush.ErrInvalidEndpoint),
		errors.Is(err, servicepush.ErrInvalidKeys):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, servicepush.ErrTooManySubscription):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
