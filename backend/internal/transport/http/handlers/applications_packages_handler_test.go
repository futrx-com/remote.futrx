package httphandlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// multipartUpload builds the request a browser form sends.
func multipartUpload(t *testing.T, field, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("close form: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/applications/packages", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	return request
}

func TestReadUploadedPackageAcceptsAMultipartFile(t *testing.T) {
	archive := []byte("PK\x03\x04 pretend archive")
	request := multipartUpload(t, packageFormField, "my-app.zip", archive)

	filename, data, err := readUploadedPackage(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if filename != "my-app.zip" {
		t.Errorf("filename = %q, want my-app.zip", filename)
	}
	if !bytes.Equal(data, archive) {
		t.Errorf("archive = %q, want %q", data, archive)
	}
}

// `curl --data-binary @app.zip` is a legitimate way to reach this route, so a
// body that is not a form is taken as the archive itself.
func TestReadUploadedPackageAcceptsARawBody(t *testing.T) {
	archive := []byte("PK\x03\x04 raw")
	request := httptest.NewRequest(
		http.MethodPost, "/api/applications/packages", bytes.NewReader(archive))
	request.Header.Set("Content-Type", "application/zip")

	filename, data, err := readUploadedPackage(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if filename != "" {
		t.Errorf("filename = %q, want empty for a raw body", filename)
	}
	if !bytes.Equal(data, archive) {
		t.Errorf("archive = %q, want %q", data, archive)
	}
}

// A form with the file under the wrong name is a client mistake worth naming,
// not an empty upload the catalog then rejects for a different reason.
func TestReadUploadedPackageNamesTheExpectedField(t *testing.T) {
	request := multipartUpload(t, "file", "my-app.zip", []byte("PK"))

	_, _, err := readUploadedPackage(httptest.NewRecorder(), request)
	if err == nil {
		t.Fatal("a form without the package field must be refused")
	}
	if !strings.Contains(err.Error(), packageFormField) {
		t.Errorf("error %q does not name the %q field", err, packageFormField)
	}
}

// The cap is enforced at the transport, so an oversized upload is refused
// before it is buffered rather than after.
func TestReadUploadedPackageRefusesAnOversizedBody(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/applications/packages",
		bytes.NewReader(make([]byte, maxPackageUpload+1)),
	)
	request.Header.Set("Content-Type", "application/zip")

	if _, _, err := readUploadedPackage(httptest.NewRecorder(), request); err == nil {
		t.Fatal("an upload past the cap must be refused")
	}
}
