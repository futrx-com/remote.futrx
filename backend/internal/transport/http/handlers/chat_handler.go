package httphandlers

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicegithistory "github.com/futrx-com/remote.futrx.com/internal/service/githistory"
	serviceworkspacefiles "github.com/futrx-com/remote.futrx.com/internal/service/workspacefiles"
	serviceworkspaceide "github.com/futrx-com/remote.futrx.com/internal/service/workspaceide"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type ChatHandler struct {
	chats     *servicechat.Service
	access    *servicechat.AccessService
	auth      *serviceauth.Service
	files     *serviceworkspacefiles.Service
	history   *servicegithistory.Service
	ide       *serviceworkspaceide.Service
	schedules *ScheduleHandler
}

func NewChatHandler(
	chats *servicechat.Service,
	access *servicechat.AccessService,
	auth *serviceauth.Service,
	files *serviceworkspacefiles.Service,
	history *servicegithistory.Service,
	ide *serviceworkspaceide.Service,
) *ChatHandler {
	return &ChatHandler{
		chats:   chats,
		access:  access,
		auth:    auth,
		files:   files,
		history: history,
		ide:     ide,
	}
}

func (h *ChatHandler) WithSchedules(schedules *ScheduleHandler) *ChatHandler {
	h.schedules = schedules
	return h
}

func (h *ChatHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/chats", h.HandleCollection)
	mux.HandleFunc("/api/chats/", h.HandleResource)
}

func (h *ChatHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	email, isAdmin, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		metas, err := h.access.List(r.Context(), email, isAdmin)
		if err != nil {
			sendChatError(w, err)
			return
		}
		if metas == nil {
			metas = []servicechat.Meta{}
		}
		httptransport.SendJSON(w, http.StatusOK, metas)

	case http.MethodPost:
		var in servicechat.CreateInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil && err != io.EOF {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		meta, err := h.access.Create(r.Context(), in, email, isAdmin)
		if err != nil {
			sendChatError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusCreated, meta)

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *ChatHandler) HandleResource(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/chats/")
	parts := strings.SplitN(rest, "/", 2)
	id := servicechat.ID(parts[0])

	email, isAdmin, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	meta, err := h.access.Get(r.Context(), id, email, isAdmin)
	if err != nil {
		sendChatError(w, err)
		return
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "events":
			h.handleEvents(w, r, id)
		case "transcript":
			h.handleTranscript(w, r, id)
		case "rewind":
			h.handleRewind(w, r, id)
		case "fork":
			h.handleFork(w, r, id)
		case "read":
			h.handleMarkRead(w, r, id)
		case "unread":
			h.handleMarkUnread(w, r, id)
		case "ide-open":
			h.handleIDEOpen(w, r, meta)
		case "media-open":
			h.handleMediaOpen(w, r, meta)
		case "files":
			h.handleFilesList(w, r, meta)
		case "files/search":
			h.handleFilesSearch(w, r, meta)
		case "files/download":
			h.handleFilesDownload(w, r, meta)
		case "files/download-folder":
			h.handleFilesDownloadFolder(w, r, meta)
		case "history/repos":
			h.handleHistoryRepos(w, r, meta)
		case "history/commits":
			h.handleHistoryCommits(w, r, meta)
		case "history/diff":
			h.handleHistoryDiff(w, r, meta)
		case "history/checkout":
			h.handleHistoryCheckout(w, r, meta)
		case "schedules":
			if h.schedules == nil {
				httptransport.SendErr(w, http.StatusNotFound, "not found")
				return
			}
			h.schedules.HandleChatCollection(w, r, meta, email, isAdmin)
		default:
			httptransport.SendErr(w, http.StatusNotFound, "not found")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		httptransport.SendJSON(w, http.StatusOK, meta)

	case http.MethodPatch:
		var in servicechat.UpdateInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		updated, err := h.chats.Update(r.Context(), id, in)
		if err != nil {
			sendChatError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		if err := h.chats.Delete(r.Context(), id); err != nil {
			sendChatError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *ChatHandler) handleEvents(w http.ResponseWriter, r *http.Request, id servicechat.ID) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	page, err := h.chats.EventPage(r.Context(), id, servicechat.EventPageQuery{
		Limit:     intQuery(r, "limit", 200),
		BeforeSeq: int64Query(r, "before", 0),
	})
	if err != nil {
		sendChatError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, page)
}

func (h *ChatHandler) handleTranscript(w http.ResponseWriter, r *http.Request, id servicechat.ID) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	page, err := h.chats.TranscriptPage(r.Context(), id, servicechat.TranscriptPageQuery{
		Limit:     intQuery(r, "limit", 0),
		BeforeSeq: int64Query(r, "before", 0),
	})
	if err != nil {
		sendChatError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, page)
}

func (h *ChatHandler) handleRewind(w http.ResponseWriter, r *http.Request, id servicechat.ID) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		BeforeT int64 `json:"beforeT"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, err := h.chats.Rewind(r.Context(), id, body.BeforeT); err != nil {
		sendChatError(w, err)
		return
	}
	page, err := h.chats.TranscriptPage(r.Context(), id, servicechat.TranscriptPageQuery{})
	if err != nil {
		sendChatError(w, err)
		return
	}
	events := make([]servicechat.Event, 0)
	for _, turn := range page.Turns {
		events = append(events, turn.Events...)
	}
	httptransport.SendJSON(w, http.StatusOK, servicechat.EventPage{
		Events:     events,
		NextBefore: page.NextBefore,
		LastSeq:    page.LastSeq,
		HasMore:    page.HasMore,
	})
}

func (h *ChatHandler) handleFork(w http.ResponseWriter, r *http.Request, id servicechat.ID) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	forked, err := h.chats.Fork(r.Context(), id)
	if err != nil {
		sendChatError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusCreated, forked)
}

func (h *ChatHandler) handleMarkRead(w http.ResponseWriter, r *http.Request, id servicechat.ID) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	meta, err := h.chats.MarkRead(r.Context(), id)
	if err != nil {
		sendChatError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, meta)
}

func (h *ChatHandler) handleMarkUnread(w http.ResponseWriter, r *http.Request, id servicechat.ID) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	meta, err := h.chats.MarkUnread(r.Context(), id)
	if err != nil {
		sendChatError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, meta)
}

func (h *ChatHandler) handleIDEOpen(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	redirectURL, err := h.ide.OpenURL(meta.Cwd, r.URL.Query().Get("path"))
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *ChatHandler) handleMediaOpen(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	media, err := h.files.OpenMedia(meta.Cwd, r.URL.Query().Get("path"))
	if err != nil {
		sendMediaOpenError(w, err)
		return
	}
	defer media.File.Close()

	if media.ContentType != "" {
		w.Header().Set("Content-Type", media.ContentType)
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": media.File.Name}))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data: blob:; media-src 'self' data: blob:; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, media.File.Name, media.File.ModTime, media.File.Content())
}

func sendMediaOpenError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceworkspacefiles.ErrUnsupportedMedia):
		httptransport.SendErr(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, serviceworkspacefiles.ErrFileNotFound):
		httptransport.SendErr(w, http.StatusNotFound, err.Error())
	default:
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	}
}

func (h *ChatHandler) caller(r *http.Request) (string, bool, error) {
	if h.auth == nil {
		return "", true, nil
	}
	return callerStateFromRequest(r.Context(), r, h.auth)
}

func intQuery(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func int64Query(r *http.Request, key string, fallback int64) int64 {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func sendChatError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, servicechat.ErrInvalidID),
		errors.Is(err, servicechat.ErrInvalidProvider),
		errors.Is(err, servicechat.ErrInvalidTmuxSession),
		errors.Is(err, servicechat.ErrInvalidRewindTimestamp):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, servicechat.ErrNotFound):
		httptransport.SendErr(w, http.StatusNotFound, "chat not found")
	case errors.Is(err, servicechat.ErrChatRunning):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, servicechat.ErrProjectMembershipRequired),
		errors.Is(err, servicechat.ErrProjectAccessDenied):
		httptransport.SendErr(w, http.StatusForbidden, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
