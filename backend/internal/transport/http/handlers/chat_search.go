package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

func (h *ChatHandler) handleSearchMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	email, isAdmin, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var query servicechat.WorkspaceSearchQuery
	if !decodeChatSearch(w, r, &query) {
		return
	}
	if err := query.Validate(); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	metas, err := h.access.List(r.Context(), email, isAdmin)
	if err != nil {
		sendChatError(w, err)
		return
	}
	projects, err := h.listSearchProjects(r.Context(), email, isAdmin)
	if err != nil {
		sendProjectError(w, err)
		return
	}
	result := servicechat.SearchWorkspace(metas, projects, query, time.Now())
	httptransport.SendJSON(w, http.StatusOK, result)
}

type workspaceMessageSearchQuery struct {
	servicechat.WorkspaceSearchSelection
	Query  string `json:"query"`
	Before string `json:"before"`
	Limit  int    `json:"limit"`
}

// HandleSearchAll searches the transcripts of visible chats. POST callers can
// additionally limit the chat set by date and facets.
func (h *ChatHandler) HandleSearchAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	email, isAdmin, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var query workspaceMessageSearchQuery
	if r.Method == http.MethodPost {
		if !decodeChatSearch(w, r, &query) {
			return
		}
		if err := query.Validate(); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		query.Query = r.URL.Query().Get("q")
		query.Before = r.URL.Query().Get("before")
		query.Limit = intQuery(r, "limit", 50)
	}
	cursor, err := servicechat.ParseChatSearchCursor(query.Before)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	metas, err := h.access.List(r.Context(), email, isAdmin)
	if err != nil {
		sendChatError(w, err)
		return
	}
	var projects []serviceproject.Meta
	if len(query.Facets["project"]) > 0 {
		projects, err = h.listSearchProjects(r.Context(), email, isAdmin)
		if err != nil {
			sendProjectError(w, err)
			return
		}
	}
	ids := servicechat.WorkspaceMessageChatIDs(metas, projects, query.WorkspaceSearchSelection)
	hits, err := h.chats.SearchChats(r.Context(), ids, query.Query, query.Limit, cursor)
	if err != nil {
		sendChatError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, hits)
}

func decodeChatSearch(w http.ResponseWriter, r *http.Request, destination any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(destination); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid search request")
		return false
	}
	return true
}

func (h *ChatHandler) listSearchProjects(ctx context.Context, email string, isAdmin bool) ([]serviceproject.Meta, error) {
	if h.searchProjects == nil {
		return nil, nil
	}
	return h.searchProjects.ListVisible(ctx, email, isAdmin)
}
