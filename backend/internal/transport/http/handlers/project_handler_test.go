package httphandlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

func TestProjectDuplicateNameErrorIsConflict(t *testing.T) {
	response := httptest.NewRecorder()

	sendProjectError(response, serviceproject.ErrNameAlreadyExists)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
	wantBody := `{"error":"project name already exists"}` + "\n"
	if response.Body.String() != wantBody {
		t.Fatalf("body = %q, want %q", response.Body.String(), wantBody)
	}
}

func TestProjectInsufficientStorageErrorIs507WithTheReason(t *testing.T) {
	response := httptest.NewRecorder()

	sendProjectError(response, fmt.Errorf("%w: 1.0 GiB free", serviceproject.ErrInsufficientStorage))

	if response.Code != http.StatusInsufficientStorage {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInsufficientStorage)
	}
	wantBody := `{"error":"not enough disk space: 1.0 GiB free"}` + "\n"
	if response.Body.String() != wantBody {
		t.Fatalf("body = %q, want %q", response.Body.String(), wantBody)
	}
}
