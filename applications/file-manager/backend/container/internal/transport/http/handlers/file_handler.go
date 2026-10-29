package httphandlers

import (
	"mime"
	"net/http"

	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/port/files/inbound"
	httptransport "futrx.local/catalog/applications/file-manager/backend/container/internal/transport/http"
)

type FileHandler struct {
	files inbound.Files
}

func NewFileHandler(files inbound.Files) *FileHandler {
	return &FileHandler{files: files}
}

func (h *FileHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/list", h.handleList)
	mux.HandleFunc("GET /api/search", h.handleSearch)
	mux.HandleFunc("GET /api/download", h.handleDownload)
	mux.HandleFunc("GET /api/archive", h.handleArchive)
}

func (h *FileHandler) handleList(w http.ResponseWriter, r *http.Request) {
	result, err := h.files.List(r.URL.Query().Get("path"))
	if err != nil {
		sendError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, listing{Path: result.Path, Entries: presentNodes(result.Entries), Truncated: result.Truncated})
}

func (h *FileHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	result, err := h.files.Search(r.URL.Query().Get("q"))
	if err != nil {
		sendError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, searchResult{Entries: presentNodes(result.Entries), Truncated: result.Truncated})
}

func (h *FileHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	file, err := h.files.OpenFile(r.URL.Query().Get("path"))
	if err != nil {
		sendError(w, err)
		return
	}
	defer file.Close()
	serveDownload(w, r, file)
}

func (h *FileHandler) handleArchive(w http.ResponseWriter, r *http.Request) {
	file, err := h.files.DownloadArchive(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		if r.Context().Err() == nil {
			sendError(w, err)
		}
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/zip")
	serveDownload(w, r, file)
}

func serveDownload(w http.ResponseWriter, r *http.Request, file *model.File) {
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	http.ServeContent(w, r, file.Name, file.ModTime, file.Content())
}
