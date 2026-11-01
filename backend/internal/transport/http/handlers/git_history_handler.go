package httphandlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicegithistory "github.com/futrx-com/remote.futrx.com/internal/service/githistory"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

func (h *ChatHandler) handleHistoryRepos(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	repositories, err := h.history.Repositories(r.Context(), meta.Cwd)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, repositories)
}

func (h *ChatHandler) handleHistoryCommits(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	commits, err := h.history.Commits(
		r.Context(),
		meta.Cwd,
		r.URL.Query().Get("repo"),
		intQuery(r, "limit", 80),
	)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, commits)
}

func (h *ChatHandler) handleHistoryDiff(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	diff, err := h.history.Diff(
		r.Context(),
		meta.Cwd,
		r.URL.Query().Get("repo"),
		r.URL.Query().Get("sha"),
	)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, diff)
}

func (h *ChatHandler) handleHistoryCheckout(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input servicegithistory.CheckoutInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&input); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	checkout, err := h.history.Checkout(r.Context(), meta.Cwd, input)
	recordAudit(
		h.audit, r,
		serviceaudit.ActionWorkspaceGitCheckout,
		auditWorkspaceTarget(meta),
		serviceaudit.Meta{"repo": input.Repo, "sha": input.SHA, "projectId": string(meta.ProjectID)},
		err,
	)
	if err != nil {
		sendGitHistoryCheckoutError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, checkout)
}

func sendGitHistoryCheckoutError(w http.ResponseWriter, err error) {
	var dirty *servicegithistory.DirtyWorkingTreeError
	if errors.As(err, &dirty) {
		httptransport.SendJSON(w, http.StatusConflict, map[string]any{
			"error":      dirty.Error(),
			"dirty":      true,
			"dirtyFiles": dirty.Files,
		})
		return
	}
	var conflict *servicegithistory.ConflictError
	if errors.As(err, &conflict) {
		httptransport.SendErr(w, http.StatusConflict, err.Error())
		return
	}
	httptransport.SendErr(w, http.StatusBadRequest, err.Error())
}
