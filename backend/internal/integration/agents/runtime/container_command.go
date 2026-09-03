package runtime

import (
	"context"
	"os/exec"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// ContainerCommandSpec is the provider-owned portion of one lxc exec command.
// The builder preserves environment precedence while keeping provider CLI
// syntax in the adapter that understands it.
type ContainerCommandSpec struct {
	ContainerName      string
	WorkingDirectory   string
	PrefixEnvironment  []string
	Secrets            []agent.ProjectSecret
	ExcludedSecrets    []string
	SuffixEnvironment  []string
	RuntimeEnvironment map[string]string
	Binary             string
	Arguments          []string
}

// BuildContainerCommand constructs, but does not start, an lxc exec command.
// Runtime keys mask same-named project secrets before invalid runtime names
// are discarded, preserving the backend-issued environment's precedence.
func BuildContainerCommand(ctx context.Context, spec ContainerCommandSpec) *exec.Cmd {
	workingDirectory := spec.WorkingDirectory
	if workingDirectory == "" {
		workingDirectory = "/workspace"
	}
	args := []string{"exec", "--cwd", workingDirectory}
	for _, entry := range spec.PrefixEnvironment {
		args = append(args, "--env", entry)
	}
	excluded := make(map[string]struct{}, len(spec.ExcludedSecrets))
	for _, key := range spec.ExcludedSecrets {
		excluded[key] = struct{}{}
	}
	for _, secret := range spec.Secrets {
		if _, skip := excluded[secret.Key]; skip {
			continue
		}
		if _, backendIssued := spec.RuntimeEnvironment[secret.Key]; backendIssued {
			continue
		}
		args = append(args, "--env", secret.Key+"="+secret.Value)
	}
	for _, entry := range spec.SuffixEnvironment {
		args = append(args, "--env", entry)
	}
	for _, entry := range agent.RuntimeEnvironment(spec.RuntimeEnvironment) {
		args = append(args, "--env", entry)
	}
	args = append(args, spec.ContainerName, "--", spec.Binary)
	args = append(args, spec.Arguments...)
	return exec.CommandContext(ctx, "lxc", args...)
}
