package httphandlers

import (
	"net/http"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

// recordAudit is the handler-layer shorthand for the actions that have no
// service call of their own to hang an entry on: sign-out, workspace file
// transfers, and session openings. Everything else is recorded in the service
// that performs it.
func recordAudit(
	recorder serviceaudit.Recorder,
	r *http.Request,
	action string,
	target serviceaudit.Target,
	meta serviceaudit.Meta,
	err error,
) {
	if recorder == nil {
		return
	}
	recorder.Record(r.Context(), serviceaudit.Result(action, target, meta, err))
}
