package httphandlers

import (
	"io"
	"net/http"
	"strings"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// Uploaded application packages: /api/applications/packages[/{id}].
//
// The catalog is server-wide, so every route here is admin-only — the same
// audience as installing a global application, and for the same reason: a
// package ships an install script, a browser extension and possibly a server
// plugin, all of which run with the server's own privileges once installed.

// maxPackageUpload bounds the whole request. The catalog's own archive limit
// is smaller; this leaves room for the multipart envelope around it and lets
// an oversized upload be refused by the transport before it is buffered.
const maxPackageUpload = 80 << 20

// packageFormField is the multipart field the SPA sends the archive in. A raw
// application/zip body is accepted too, for `curl --data-binary`.
const packageFormField = "package"

func (h *ApplicationsHandler) handlePackages(w http.ResponseWriter, r *http.Request, rest string) {
	if !h.requireAdmin(w, r) {
		return
	}
	id := strings.TrimPrefix(rest, "packages")
	id = strings.TrimPrefix(id, "/")
	if id != "" {
		if r.Method != http.MethodDelete {
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.removePackage(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		packages, err := h.apps.Packages(r.Context())
		if err != nil {
			sendAppError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, packages)
	case http.MethodPost:
		h.uploadPackage(w, r)
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// removePackage deletes a package, optionally uninstalling the copies that are
// still installed. Cascading is opt-in through ?uninstall=true rather than the
// default, so a plain DELETE can never destroy a database's container as a side
// effect of tidying the catalog.
func (h *ApplicationsHandler) removePackage(w http.ResponseWriter, r *http.Request, id string) {
	uninstall := r.URL.Query().Get("uninstall") == "true"
	removed, err := h.apps.RemovePackage(r.Context(), serviceapplications.RemovePackageRequest{
		ID:                 id,
		UninstallInstalled: uninstall,
	})
	if err != nil {
		sendAppError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"uninstalled": orEmptyInstalls(removed),
	})
}

func orEmptyInstalls(installs []serviceapplications.PackageInstall) []serviceapplications.PackageInstall {
	if installs == nil {
		return []serviceapplications.PackageInstall{}
	}
	return installs
}

func (h *ApplicationsHandler) uploadPackage(w http.ResponseWriter, r *http.Request) {
	filename, archive, err := readUploadedPackage(w, r)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	email, _ := callerEmailFromRequest(r, h.auth)
	pkg, err := h.apps.UploadPackage(r.Context(), serviceapplications.PackageUpload{
		Filename: filename,
		Data:     archive,
		Actor:    email,
	})
	if err != nil {
		sendAppError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusCreated, pkg)
}

// readUploadedPackage accepts the archive either as a multipart file — what a
// browser form sends — or as the raw request body.
func readUploadedPackage(w http.ResponseWriter, r *http.Request) (string, []byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPackageUpload)

	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		archive, err := io.ReadAll(r.Body)
		if err != nil {
			return "", nil, errUploadTooLarge(err)
		}
		return "", archive, nil
	}

	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return "", nil, errUploadTooLarge(err)
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, header, err := r.FormFile(packageFormField)
	if err != nil {
		return "", nil, errNoPackageField
	}
	defer file.Close()
	archive, err := io.ReadAll(file)
	if err != nil {
		return "", nil, errUploadTooLarge(err)
	}
	return header.Filename, archive, nil
}

type uploadError string

func (e uploadError) Error() string { return string(e) }

const errNoPackageField = uploadError(
	"no package in request: attach the .zip as the \"" + packageFormField + "\" field")

func errUploadTooLarge(err error) error {
	return uploadError("upload too large or malformed: " + err.Error())
}
