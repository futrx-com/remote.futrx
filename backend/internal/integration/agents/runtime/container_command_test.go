package runtime

import (
	"context"
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestBuildContainerCommandPreservesEnvironmentOrderAndPrecedence(t *testing.T) {
	command := BuildContainerCommand(context.Background(), ContainerCommandSpec{
		ContainerName:     "project-container",
		PrefixEnvironment: []string{"HOME=/root", "PROVIDER_HOME=/root/.provider"},
		Secrets: []agent.ProjectSecret{
			{Key: "FIRST_SECRET", Value: "first"},
			{Key: "EXCLUDED_SECRET", Value: "excluded"},
			{Key: "RUNTIME_VALUE", Value: "attacker"},
			{Key: "invalid-name", Value: "masked"},
		},
		ExcludedSecrets:   []string{"EXCLUDED_SECRET"},
		SuffixEnvironment: []string{"EXCLUDED_SECRET="},
		RuntimeEnvironment: map[string]string{
			"RUNTIME_VALUE": "trusted",
			"ALPHA_VALUE":   "sorted-first",
			"invalid-name":  "discarded",
		},
		Binary:    "provider-cli",
		Arguments: []string{"run", "--json"},
	})

	want := []string{
		"lxc", "exec", "--cwd", "/workspace",
		"--env", "HOME=/root",
		"--env", "PROVIDER_HOME=/root/.provider",
		"--env", "FIRST_SECRET=first",
		"--env", "EXCLUDED_SECRET=",
		"--env", "ALPHA_VALUE=sorted-first",
		"--env", "RUNTIME_VALUE=trusted",
		"project-container", "--", "provider-cli", "run", "--json",
	}
	if !slices.Equal(command.Args, want) {
		t.Fatalf("container command\n got: %#v\nwant: %#v", command.Args, want)
	}
}
