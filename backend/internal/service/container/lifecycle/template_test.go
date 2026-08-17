package lifecycle

import (
	"context"
	"errors"
	"slices"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type recordingTemplates struct {
	events *[]string
	// image is returned by ImageFor for every template except the default.
	image string
}

func (r recordingTemplates) ImageFor(_ context.Context, template string) string {
	*r.events = append(*r.events, "template image "+template)
	if template == serviceproject.DefaultTemplate {
		return ""
	}
	return r.image
}

func (r recordingTemplates) Ensure(_ context.Context, container, template string) <-chan struct{} {
	*r.events = append(*r.events, "template ensure "+container+" "+template)
	done := make(chan struct{})
	close(done)
	return done
}

func newTemplateService(
	runtime *recordingRuntime,
	events *[]string,
	templates TemplateProvisioner,
) *Service {
	return NewService(
		runtime,
		"local:remote-base",
		recordingWorkspace{events: events},
		recordingResources{events: events},
		recordingProvisioner{events: events},
		templates,
	)
}

func TestEnsureLaunchesFromThePrebuiltTemplateImageWhenOneExists(t *testing.T) {
	tests := []struct {
		name      string
		template  string
		available string
		wantImage string
	}{
		{
			name:      "template image published",
			template:  "wordpress",
			available: "futrx-remote-wordpress-base",
			wantImage: "futrx-remote-wordpress-base",
		},
		{
			name:      "no template image falls back to the shared base",
			template:  "wordpress",
			wantImage: "local:remote-base",
		},
		{
			name:      "the default template always uses the shared base",
			template:  "blank",
			available: "futrx-remote-wordpress-base",
			wantImage: "local:remote-base",
		},
		{
			name:      "metadata without a template behaves like the default",
			template:  "",
			available: "futrx-remote-wordpress-base",
			wantImage: "local:remote-base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []string
			runtime := &recordingRuntime{
				events: &events, available: true, state: serviceproject.ContainerStateMissing,
			}
			project := testProject(t)
			project.Template = tt.template
			service := newTemplateService(
				runtime, &events, recordingTemplates{events: &events, image: tt.available},
			)

			if err := service.Ensure(context.Background(), project); err != nil {
				t.Fatal(err)
			}
			want := "runtime init " + tt.wantImage + " project-1"
			if !slices.Contains(events, want) {
				t.Fatalf("missing %q in %q", want, events)
			}
		})
	}
}

func TestEnsureRunsTemplateProvisioningOnEveryConvergence(t *testing.T) {
	// The marker file lives in the disposable rootfs, so provisioning must be
	// offered on every convergence: a replaced container has to be
	// re-provisioned and an interrupted run has to be retried. The template
	// service itself is the one that short-circuits on the marker.
	tests := []struct {
		name  string
		state serviceproject.ContainerState
		ready bool
	}{
		{name: "newly created container", state: serviceproject.ContainerStateMissing},
		{name: "already running and healthy", state: serviceproject.ContainerStateRunning, ready: true},
		{name: "stopped container being started", state: serviceproject.ContainerStateStopped, ready: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []string
			project := testProject(t)
			project.Template = "wordpress"
			runtime := &recordingRuntime{events: &events, available: true, state: tt.state}
			if tt.ready {
				runtime.devices = expectedDisks(project)
			}
			service := newTemplateService(runtime, &events, recordingTemplates{events: &events})

			if err := service.Ensure(context.Background(), project); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(events, "template ensure project-1 wordpress") {
				t.Fatalf("template provisioning was not offered: %q", events)
			}
		})
	}
}

func TestEnsureWithoutATemplateProviderKeepsTheSharedBaseImage(t *testing.T) {
	var events []string
	runtime := &recordingRuntime{
		events: &events, available: true, state: serviceproject.ContainerStateMissing,
	}
	project := testProject(t)
	project.Template = "wordpress"

	// The five-argument construction (no templates) must behave exactly as it
	// did before templates existed.
	if err := newTestService(runtime, &events).Ensure(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(events, "runtime init local:remote-base project-1") {
		t.Fatalf("missing base-image init: %q", events)
	}
	for _, event := range events {
		if event == "template ensure project-1 wordpress" {
			t.Fatalf("template provisioning ran without a provider: %q", events)
		}
	}
}

func TestEnsureSkipsTemplateProvisioningWhenConvergenceFails(t *testing.T) {
	var events []string
	runtime := &recordingRuntime{
		events: &events, available: true, state: serviceproject.ContainerStateMissing,
		attachErr: errors.New("attach failed"),
	}
	project := testProject(t)
	project.Template = "wordpress"
	service := newTemplateService(runtime, &events, recordingTemplates{events: &events})

	if err := service.Ensure(context.Background(), project); err == nil {
		t.Fatal("Ensure() = nil, want the attach failure")
	}
	for _, event := range events {
		if event == "template ensure project-1 wordpress" {
			t.Fatalf("template provisioning ran against a rolled-back container: %q", events)
		}
	}
}
