package config

import (
	"context"
	"io"
	"testing"
)

type stubContainerRunner struct{}

func (stubContainerRunner) Available() bool { return true }

func (stubContainerRunner) Run(context.Context, ...string) (string, error) { return "", nil }

func (stubContainerRunner) RunStdin(context.Context, io.Reader, ...string) (string, error) {
	return "", nil
}

func TestContainerStackExposesCompleteCapabilityBundles(t *testing.T) {
	stack := NewContainerStack(stubContainerRunner{}, nil, ContainerStackOptions{
		AgentInstructions: []byte("test instructions"),
	})

	projects := stack.ProjectDependencies()
	if projects.Lifecycle != stack.Lifecycle ||
		projects.Environment != stack.Environment ||
		projects.Inspector != stack.Inspection ||
		projects.Network != stack.Network ||
		projects.Listeners != stack.Listeners ||
		projects.Browser != stack.Browser {
		t.Fatal("project dependencies do not match the composed container capabilities")
	}

	agents := stack.AgentDependencies()
	if err := agents.Validate(); err != nil {
		t.Fatalf("agent dependencies are incomplete: %v", err)
	}
	if agents.CLI != stack.CLI ||
		agents.Credentials != stack.Credentials ||
		agents.Workspace != stack.Workspace ||
		agents.RuntimeAssets != stack.RuntimeAssets ||
		agents.Browser != stack.Browser ||
		agents.Lifecycle != stack.Lifecycle {
		t.Fatal("agent dependencies do not match the composed container capabilities")
	}
}
