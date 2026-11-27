package tests

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/applicationtest"
)

func TestBackendBuildsAndDescribesRoutes(t *testing.T) {
	applicationtest.BackendRoutes(t, "scheduled-tasks", []string{"* tasks/*", "GET health", "GET tasks", "POST tasks"})
}
