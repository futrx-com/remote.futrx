package project

import (
	"context"
	"errors"
	"testing"
)

func TestCreatePassesGitURLThroughToContainerEnsure(t *testing.T) {
	repo := &startTestRepository{}
	lifecycle := &startTestLifecycle{state: ContainerStateMissing}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, nil)

	gitURL := "https://example.com/octocat/hello-world.git"
	got, err := service.Create(context.Background(), CreateInput{Name: "hello world", GitURL: gitURL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.GitURL != gitURL {
		t.Fatalf("Create() meta.GitURL = %q, want %q", got.GitURL, gitURL)
	}
	if lifecycle.lastEnsured.GitURL != gitURL {
		t.Fatalf("Ensure() was called with GitURL = %q, want %q", lifecycle.lastEnsured.GitURL, gitURL)
	}
}

func TestCreateRejectsInvalidGitURLBeforeProvisioning(t *testing.T) {
	repo := &startTestRepository{}
	lifecycle := &startTestLifecycle{state: ContainerStateMissing}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, nil)

	for name, url := range map[string]string{
		"ssh shorthand": "git@github.com:octocat/hello-world.git",
		"ssh scheme":    "ssh://git@github.com/octocat/hello-world.git",
		"plain http":    "http://example.com/octocat/hello-world.git",
		"garbage":       "not a url",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := service.Create(context.Background(), CreateInput{Name: "hello world", GitURL: url}, "")
			if !errors.Is(err, ErrInvalidGitURL) {
				t.Fatalf("Create() error = %v, want %v", err, ErrInvalidGitURL)
			}
		})
	}
	if lifecycle.launchCalls != 0 {
		t.Fatalf("Ensure() was called %d times for invalid input, want 0", lifecycle.launchCalls)
	}
}
