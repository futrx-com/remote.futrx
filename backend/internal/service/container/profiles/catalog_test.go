package profiles

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

func TestCatalogReturnsDefensiveSnapshots(t *testing.T) {
	configured := []provisioning.Profile{{
		ID: "agent",
		Credentials: provisioning.CredentialSpec{
			Files: []provisioning.CredentialFile{{HostPath: "original"}},
		},
	}}
	catalog := NewCatalog(configured)
	configured[0].Credentials.Files[0].HostPath = "mutated input"

	first := catalog.Snapshot()
	first[0].Credentials.Files[0].HostPath = "mutated snapshot"
	second := catalog.Snapshot()

	if got := second[0].Credentials.Files[0].HostPath; got != "original" {
		t.Fatalf("snapshot host path = %q, want original", got)
	}
}
