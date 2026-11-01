package httphandlers

import (
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
	"mime"
	"net/http"
)

func (h *ChatHandler) handleFilesDownload(w http.ResponseWriter, r *http.Request, meta servicechat.Meta) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	file, err := h.files.OpenFile(meta.Cwd, r.URL.Query().Get("path"))
	if err != nil {
		sendMediaOpenError(w, err)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, file.Name, file.ModTime, file.Content())
}
