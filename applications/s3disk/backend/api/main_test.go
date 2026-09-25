package api

import (
	"testing"

	appLifecycle "futrx.local/catalog/applications/s3disk/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func testBackend(t *testing.T) *backend {
	return testBackendWithPushEvents(t, &recordingPushEvents{})
}

type recordingPushEvents struct {
	outcomes []appLifecycle.PushOutcome
	err      error
}

func (e *recordingPushEvents) Completed(outcome appLifecycle.PushOutcome) error {
	e.outcomes = append(e.outcomes, outcome)
	return e.err
}

func testBackendWithPushEvents(t *testing.T, pushes appLifecycle.PushEvents) *backend {
	t.Helper()
	b := newBackend(appLifecycle.NewOperations(), pushes)
	if err := b.Init(testInstance()); err != nil {
		t.Fatal(err)
	}
	return b
}

func testInstance() applications.Instance {
	return applications.Instance{
		ApplicationID: "s3disk",
		Service:       "s3disk",
		Scope:         "project",
		ContainerName: "project-test",
		Env: map[string]string{
			"AWS_SECRET_ACCESS_KEY": "private-value",
			"S3DISK_MOUNTPOINT":     "/workspace/s3",
			"S3DISK_UPLOADS_DIR":    "uploads",
		},
	}
}
