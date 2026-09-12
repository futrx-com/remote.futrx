package httphandlers

// Admin CRUD for the platform-wide global skills library. Like the users
// routes, every entry point re-checks `IsAdmin` because the shared API gate
// only proves the caller is a registered user.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceskills "github.com/futrx-com/remote.futrx.com/internal/service/skills"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

const (
	globalSkillsRoute   = "/api/admin/skills-global"
	globalSkillsPrefix  = globalSkillsRoute + "/"
	globalSkillsImport  = "import"
	maxGlobalSkillBody  = 4 << 20
	globalSkillZipMedia = "application/zip"
)

// GlobalSkillHandler exposes the global skills library to administrators.
type GlobalSkillHandler struct {
	global *serviceskills.GlobalService
	auth   *serviceauth.Service
}

func NewGlobalSkillHandler(
	global *serviceskills.GlobalService,
	auth *serviceauth.Service,
) *GlobalSkillHandler {
	return &GlobalSkillHandler{global: global, auth: auth}
}

func (h *GlobalSkillHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc(globalSkillsRoute, h.HandleCollection)
	mux.HandleFunc(globalSkillsPrefix, h.HandleResource)
}

func (h *GlobalSkillHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		list, err := h.global.List(r.Context())
		if err != nil {
			sendGlobalSkillError(w, err)
			return
		}
		if list == nil {
			list = []serviceskills.GlobalSkill{}
		}
		httptransport.SendJSON(w, http.StatusOK, list)

	case http.MethodPost:
		input, err := decodeGlobalSkillPayload(r)
		if err != nil {
			sendGlobalSkillError(w, err)
			return
		}
		created, err := h.global.Create(r.Context(), serviceskills.GlobalInput{
			Name:     input.Name,
			Files:    input.Files,
			AlwaysOn: input.AlwaysOn != nil && *input.AlwaysOn,
		})
		if err != nil {
			sendGlobalSkillError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusCreated, created)

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *GlobalSkillHandler) HandleResource(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, globalSkillsPrefix)
	rest = strings.Trim(rest, "/")
	if rest == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "missing skill name")
		return
	}
	if strings.Contains(rest, "/") {
		httptransport.SendErr(w, http.StatusNotFound, "not found")
		return
	}
	name, err := url.PathUnescape(rest)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid skill path")
		return
	}

	if name == globalSkillsImport {
		h.handleImport(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		skill, err := h.global.Get(r.Context(), name)
		if err != nil {
			sendGlobalSkillError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, skill)

	case http.MethodPut:
		input, err := decodeGlobalSkillPayload(r)
		if err != nil {
			sendGlobalSkillError(w, err)
			return
		}
		updated, err := h.global.Update(r.Context(), name, serviceskills.GlobalUpdate{
			Files:    input.Files,
			AlwaysOn: input.AlwaysOn,
		})
		if err != nil {
			sendGlobalSkillError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		if err := h.global.Delete(r.Context(), name); err != nil {
			sendGlobalSkillError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleImport copies an existing project skill directory into the library.
func (h *GlobalSkillHandler) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		ProjectID string `json:"projectId"`
		Skill     string `json:"skill"`
		Name      string `json:"name"`
		AlwaysOn  bool   `json:"alwaysOn"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	projectID := serviceproject.ID(strings.TrimSpace(body.ProjectID))
	if projectID == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "projectId is required")
		return
	}
	imported, err := h.global.ImportFromProject(
		r.Context(),
		projectID,
		body.Skill,
		body.Name,
		body.AlwaysOn,
	)
	if err != nil {
		sendGlobalSkillError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusCreated, imported)
}

func (h *GlobalSkillHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.global == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "global skills unavailable")
		return false
	}
	if h.auth == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "authentication unavailable")
		return false
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	if ok, _ := h.auth.IsAdmin(r.Context(), email); !ok {
		httptransport.SendErr(w, http.StatusForbidden, "admin only")
		return false
	}
	return true
}

// globalSkillPayload is the shared body of a create and an update. Files is
// nil when the caller only wants to flip a flag.
type globalSkillPayload struct {
	Name     string
	Files    map[string]string
	AlwaysOn *bool
}

// decodeGlobalSkillPayload accepts either a JSON files map — the shape the
// rest of this API uses — or a zip archive holding a whole skill folder.
func decodeGlobalSkillPayload(r *http.Request) (globalSkillPayload, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if mediaType, _, ok := strings.Cut(contentType, ";"); ok {
		contentType = strings.TrimSpace(mediaType)
	}

	if contentType == globalSkillZipMedia || contentType == "application/x-zip-compressed" {
		return decodeGlobalSkillZip(r)
	}

	var body struct {
		Name     string            `json:"name"`
		Files    map[string]string `json:"files"`
		AlwaysOn *bool             `json:"alwaysOn"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxGlobalSkillBody)).Decode(&body); err != nil {
		return globalSkillPayload{}, errInvalidGlobalSkillBody
	}
	return globalSkillPayload{Name: body.Name, Files: body.Files, AlwaysOn: body.AlwaysOn}, nil
}

// decodeGlobalSkillZip flattens an uploaded archive into the same files map a
// JSON body carries. A single wrapping directory (the common result of
// zipping a skill folder) is stripped so `SKILL.md` lands at the root.
func decodeGlobalSkillZip(r *http.Request) (globalSkillPayload, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxGlobalSkillBody))
	if err != nil {
		return globalSkillPayload{}, errInvalidGlobalSkillBody
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return globalSkillPayload{}, errInvalidGlobalSkillArchive
	}

	files := map[string]string{}
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.UncompressedSize64 > serviceskills.MaxGlobalSkillFileBytes {
			return globalSkillPayload{}, serviceskills.ErrGlobalSkillTooLarge
		}
		if len(files) >= serviceskills.MaxGlobalSkillFiles {
			return globalSkillPayload{}, serviceskills.ErrGlobalSkillTooLarge
		}
		reader, err := entry.Open()
		if err != nil {
			return globalSkillPayload{}, errInvalidGlobalSkillArchive
		}
		content, err := io.ReadAll(io.LimitReader(reader, serviceskills.MaxGlobalSkillFileBytes+1))
		reader.Close()
		if err != nil {
			return globalSkillPayload{}, errInvalidGlobalSkillArchive
		}
		if len(content) > serviceskills.MaxGlobalSkillFileBytes {
			return globalSkillPayload{}, serviceskills.ErrGlobalSkillTooLarge
		}
		files[entry.Name] = string(content)
	}

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	stripped, prefix := stripArchiveRoot(files)
	if name == "" {
		name = prefix
	}
	payload := globalSkillPayload{Name: name, Files: stripped}
	// Only an explicit query parameter touches the flag, so re-uploading an
	// archive over an existing skill does not silently unpin it.
	if raw := r.URL.Query().Get("alwaysOn"); raw != "" {
		alwaysOn := raw == "true" || raw == "1"
		payload.AlwaysOn = &alwaysOn
	}
	return payload, nil
}

// stripArchiveRoot removes a shared top-level directory from every entry and
// reports the directory name, which doubles as a default skill name.
func stripArchiveRoot(files map[string]string) (map[string]string, string) {
	if _, ok := files["SKILL.md"]; ok {
		return files, ""
	}
	prefix := ""
	for candidate := range files {
		head, _, ok := strings.Cut(strings.TrimPrefix(candidate, "./"), "/")
		if !ok {
			return files, ""
		}
		if prefix == "" {
			prefix = head
			continue
		}
		if prefix != head {
			return files, ""
		}
	}
	if prefix == "" {
		return files, ""
	}
	stripped := make(map[string]string, len(files))
	for candidate, content := range files {
		stripped[strings.TrimPrefix(strings.TrimPrefix(candidate, "./"), prefix+"/")] = content
	}
	return stripped, prefix
}

var (
	errInvalidGlobalSkillBody    = errors.New("invalid json")
	errInvalidGlobalSkillArchive = errors.New("invalid zip archive")
)

func sendGlobalSkillError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errInvalidGlobalSkillBody),
		errors.Is(err, errInvalidGlobalSkillArchive),
		errors.Is(err, serviceskills.ErrInvalidGlobalSkillName),
		errors.Is(err, serviceskills.ErrInvalidGlobalSkillFile),
		errors.Is(err, serviceskills.ErrMissingSkillManifest),
		errors.Is(err, serviceskills.ErrGlobalSkillTooLarge):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, serviceskills.ErrGlobalSkillNotFound),
		errors.Is(err, serviceskills.ErrProjectNotFound),
		errors.Is(err, serviceskills.ErrProjectSkillNotFound):
		httptransport.SendErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, serviceskills.ErrGlobalSkillExists):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, serviceskills.ErrGlobalLibraryUnavailable),
		errors.Is(err, serviceskills.ErrProjectLookupUnavailable):
		httptransport.SendErr(w, http.StatusServiceUnavailable, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
