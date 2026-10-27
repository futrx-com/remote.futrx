package httphandlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

func TestProjectReservedSeparatorReturnsBadRequest(t *testing.T) {
	response := httptest.NewRecorder()
	sendProjectError(response, serviceproject.ErrReservedNameSeparator)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "consecutive hyphens") {
		t.Fatal(response.Code, response.Body.String())
	}
}
