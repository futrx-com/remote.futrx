package tests

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/applicationtest"
)

func TestBackendBuildsAndDescribesRoutes(t *testing.T) {
	applicationtest.BackendRoutes(t, "code-server", []string{"GET settings", "POST settings"})
}
