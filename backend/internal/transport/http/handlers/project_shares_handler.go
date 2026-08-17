package httphandlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// shareResponse is the metadata view of a share link. The token and its digest
// stay out of every response except the one that creates the link.
type shareResponse struct {
	ID        string `json:"id"`
	Port      int    `json:"port"`
	Label     string `json:"label,omitempty"`
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
	// URL is populated only by the create response — this is the single time
	// the plaintext token is ever shown.
	URL string `json:"url,omitempty"`
}

// handleShares serves /api/projects/{id}/shares[/{shareId}]. Project
// membership was already established by HandleResource; admins reach every
// project, so admin revocation needs no extra branch here.
func (h *ProjectHandler) handleShares(
	w http.ResponseWriter,
	r *http.Request,
	id serviceproject.ID,
	parts []string,
) {
	if h.shares == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, serviceshare.ErrUnavailable.Error())
		return
	}

	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			h.listShares(w, r, id)
		case http.MethodPost:
			h.createShare(w, r, id)
		default:
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if r.Method != http.MethodDelete {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	shareID, err := url.PathUnescape(strings.Trim(parts[2], "/"))
	if err != nil || shareID == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "missing share id")
		return
	}
	if err := h.shares.Revoke(r.Context(), id, serviceshare.ID(shareID)); err != nil {
		sendShareError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ProjectHandler) listShares(w http.ResponseWriter, r *http.Request, id serviceproject.ID) {
	shares, err := h.shares.List(r.Context(), id)
	if err != nil {
		sendShareError(w, err)
		return
	}
	out := make([]shareResponse, 0, len(shares))
	for _, record := range shares {
		out = append(out, shareMetadata(record))
	}
	httptransport.SendJSON(w, http.StatusOK, out)
}

func (h *ProjectHandler) createShare(w http.ResponseWriter, r *http.Request, id serviceproject.ID) {
	var body serviceshare.CreateInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	email, _, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	created, err := h.shares.Create(r.Context(), id, body, email)
	if err != nil {
		sendShareError(w, err)
		return
	}
	response := shareMetadata(created.Share)
	response.URL = h.shareURL(created.Slug, created.Share.Port, created.Token)
	httptransport.SendJSON(w, http.StatusCreated, response)
}

// shareURL is the one link handed to the outside viewer. It targets the same
// preview host the platform already uses, with the token as a query parameter
// that /auth/verify swaps for a cookie on first load.
func (h *ProjectHandler) shareURL(slug string, port int, token string) string {
	if slug == "" || h.publicHostname == "" {
		return ""
	}
	return "https://" + slug + "--" + strconv.Itoa(port) + ".dev." + h.publicHostname +
		"/?" + shareQueryParam + "=" + url.QueryEscape(token)
}

func shareMetadata(record serviceshare.Share) shareResponse {
	return shareResponse{
		ID:        string(record.ID),
		Port:      record.Port,
		Label:     record.Label,
		CreatedBy: record.CreatedBy,
		CreatedAt: record.CreatedAt,
		ExpiresAt: record.ExpiresAt,
	}
}

func sendShareError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceshare.ErrInvalidPort),
		errors.Is(err, serviceshare.ErrPortNotShareable),
		errors.Is(err, serviceshare.ErrInvalidTTL):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, serviceshare.ErrTooManyShares):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, serviceshare.ErrNotFound):
		httptransport.SendErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, serviceshare.ErrUnavailable):
		httptransport.SendErr(w, http.StatusServiceUnavailable, err.Error())
	default:
		sendProjectError(w, err)
	}
}
