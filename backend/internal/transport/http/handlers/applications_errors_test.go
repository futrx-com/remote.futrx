package httphandlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// Every error the applications service defines has to reach the caller as the
// kind of answer it is. A 500 tells the user that Remote broke and that
// retrying might help, so anything the caller could have avoided — asking a
// portless app to change its port, say — must not arrive as one.
func TestSendAppErrorMapsServiceErrorsToStatuses(t *testing.T) {
	cases := map[error]int{
		serviceapplications.ErrNotFound:         http.StatusNotFound,
		serviceapplications.ErrBackendAccess:    http.StatusForbidden,
		serviceapplications.ErrAlreadyInstalled: http.StatusConflict,
		serviceapplications.ErrNotSupported:     http.StatusUnprocessableEntity,
		serviceapplications.ErrScope:            http.StatusBadRequest,
		serviceapplications.ErrUnavailable:      http.StatusServiceUnavailable,
		errors.New("disk on fire"):              http.StatusInternalServerError,
	}
	for err, want := range cases {
		// Wrapped, because that is how the service returns them: the sentinel
		// carries the class and the wrapping carries which image it was about.
		wrapped := fmt.Errorf("%w: some-image", err)
		recorder := httptest.NewRecorder()
		sendAppError(recorder, wrapped)
		if recorder.Code != want {
			t.Errorf("%v: status = %d, want %d", err, recorder.Code, want)
		}
	}
}
