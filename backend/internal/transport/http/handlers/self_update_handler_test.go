package httphandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReleaseNotesRequiresAdminAndGet(t *testing.T) {
	mux := http.NewServeMux()
	NewSelfUpdateHandler(nil, nil).RegisterRoutes(mux)
	for _, tc := range []struct {
		method string
		want   int
	}{
		{http.MethodGet, http.StatusForbidden},
		{http.MethodPost, http.StatusMethodNotAllowed},
	} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(tc.method, "/api/admin/update/release-notes?tag=0.25.1", nil))
		if recorder.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.method, recorder.Code, tc.want)
		}
	}
}
