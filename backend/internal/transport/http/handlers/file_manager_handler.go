package httphandlers

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceworkspacefiles "github.com/futrx-com/remote.futrx.com/internal/service/workspacefiles"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

func (h *ChatHandler) handleFilesList(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	listing, err := h.files.List(meta.Cwd, r.URL.Query().Get("path"))
	if err != nil {
		sendWorkspaceFileError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, listing)
}

func (h *ChatHandler) handleFilesSearch(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := h.files.Search(meta.Cwd, r.URL.Query().Get("q"))
	if err != nil {
		sendWorkspaceFileError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, result)
}

func (h *ChatHandler) handleFilesDownload(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := r.URL.Query().Get("path")
	file, err := h.files.OpenFile(meta.Cwd, path)
	recordAudit(
		h.audit, r,
		serviceaudit.ActionWorkspaceFileDownload,
		auditWorkspaceTarget(meta),
		serviceaudit.Meta{"path": path, "projectId": string(meta.ProjectID)},
		err,
	)
	if err != nil {
		sendWorkspaceFileError(w, err)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, file.Name, file.ModTime, file.Content())
}

func (h *ChatHandler) handleFilesDownloadFolder(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := r.URL.Query().Get("path")
	archive, err := h.files.PrepareArchive(meta.Cwd, path)
	recordAudit(
		h.audit, r,
		serviceaudit.ActionWorkspaceArchiveDownload,
		auditWorkspaceTarget(meta),
		serviceaudit.Meta{"path": path, "projectId": string(meta.ProjectID)},
		err,
	)
	if err != nil {
		sendWorkspaceFileError(w, err)
		return
	}

	spooled, err := workspaceArchiveSpooler.prepare(r.Context(), func(destination io.Writer) error {
		return h.files.WriteArchive(r.Context(), archive, destination)
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		sendWorkspaceFileError(w, err)
		return
	}
	defer spooled.close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": archive.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, archive.Name, time.Time{}, spooled.file)
}

func sendWorkspaceFileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errWorkspaceArchiveTooLarge):
		httptransport.SendErr(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, serviceworkspacefiles.ErrInvalidPath):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, serviceworkspacefiles.ErrFileNotFound),
		errors.Is(err, serviceworkspacefiles.ErrFolderNotFound):
		httptransport.SendErr(w, http.StatusNotFound, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
