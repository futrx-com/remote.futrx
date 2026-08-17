package image

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func newTemplateBuilder(runtime *recordingRuntime) *Builder {
	builder := NewBuilder(runtime, &recordingProfileSource{profiles: configuredProfiles()},
		"browser-install", []byte("code-server-install"), nil)
	builder.networkWarmup = 0
	return builder
}

func TestBuildTemplateLayersOnTopOfThePublishedBaseImage(t *testing.T) {
	runtime := &recordingRuntime{available: true}
	builder := newTemplateBuilder(runtime)

	err := builder.BuildTemplate(context.Background(), TemplateSpec{
		Name:    "wordpress",
		Alias:   "futrx-remote-wordpress-base",
		Program: "install wordpress",
	})
	if err != nil {
		t.Fatalf("BuildTemplate: %v", err)
	}
	want := []string{
		"available",
		"delete " + templateImageBuilderName,
		// The builder starts from the shared base image, not from Ubuntu:
		// a template image is base + one script, never a repeat of the
		// base recipe.
		"launch " + Alias + " " + templateImageBuilderName,
		"script " + templateImageBuilderName + " " + ipv4EgressProbe,
		"script " + templateImageBuilderName + " install wordpress",
		"stop " + templateImageBuilderName,
		"publish " + templateImageBuilderName + " futrx-remote-wordpress-base " +
			"futrx remote wordpress template: " + Alias + " + wordpress stack",
		"delete " + templateImageBuilderName,
	}
	assertEvents(t, runtime.events, want)
}

func TestBuildTemplateHonoursAnExplicitBaseAndDescription(t *testing.T) {
	runtime := &recordingRuntime{available: true}

	err := newTemplateBuilder(runtime).BuildTemplate(context.Background(), TemplateSpec{
		Name:        "wordpress",
		Alias:       "custom-alias",
		BaseAlias:   "custom-base",
		Program:     "install",
		Description: "custom description",
	})
	if err != nil {
		t.Fatalf("BuildTemplate: %v", err)
	}
	if !containsEvent(runtime.events, "launch custom-base "+templateImageBuilderName) {
		t.Fatalf("events = %q", runtime.events)
	}
	if !containsEvent(runtime.events,
		"publish "+templateImageBuilderName+" custom-alias custom description") {
		t.Fatalf("events = %q", runtime.events)
	}
}

func TestBuildTemplateRejectsIncompleteSpecs(t *testing.T) {
	tests := []struct {
		name    string
		spec    TemplateSpec
		wantErr string
	}{
		{
			name:    "no name",
			spec:    TemplateSpec{Alias: "a", Program: "p"},
			wantErr: "template name and an alias",
		},
		{
			name:    "no alias",
			spec:    TemplateSpec{Name: "n", Program: "p"},
			wantErr: "template name and an alias",
		},
		{
			name:    "nothing to bake",
			spec:    TemplateSpec{Name: "blank", Alias: "a"},
			wantErr: "no provisioning to bake",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &recordingRuntime{available: true}
			err := newTemplateBuilder(runtime).BuildTemplate(context.Background(), tt.spec)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("BuildTemplate() = %v, want an error containing %q", err, tt.wantErr)
			}
			// A rejected spec must not launch anything.
			for _, event := range runtime.events {
				if strings.HasPrefix(event, "launch ") {
					t.Fatalf("events = %q", runtime.events)
				}
			}
		})
	}
}

func TestBuildTemplateCleansUpTheBuilderOnFailure(t *testing.T) {
	runtime := &recordingRuntime{
		available: true,
		scriptResponses: []runtimeResponse{
			{}, // IPv4 egress probe succeeds
			{output: "apt exploded", err: errors.New("exit 1")},
		},
	}

	err := newTemplateBuilder(runtime).BuildTemplate(context.Background(), TemplateSpec{
		Name: "wordpress", Alias: "futrx-remote-wordpress-base", Program: "install",
	})
	if err == nil || !strings.Contains(err.Error(), "apt exploded") {
		t.Fatalf("BuildTemplate() = %v, want the script output preserved", err)
	}
	if runtime.events[len(runtime.events)-1] != "delete "+templateImageBuilderName {
		t.Fatalf("builder was not cleaned up: %q", runtime.events)
	}
	for _, event := range runtime.events {
		if strings.HasPrefix(event, "publish ") {
			t.Fatalf("a failed build published an image: %q", runtime.events)
		}
	}
}

func TestBuildTemplateRequiresAnAvailableRuntime(t *testing.T) {
	runtime := &recordingRuntime{}
	err := newTemplateBuilder(runtime).BuildTemplate(context.Background(), TemplateSpec{
		Name: "wordpress", Alias: "a", Program: "install",
	})
	if err == nil || !strings.Contains(err.Error(), "lxc CLI not found") {
		t.Fatalf("BuildTemplate() = %v", err)
	}
}

func TestTimeoutOverridesIgnoreNonPositiveValues(t *testing.T) {
	builder := newTemplateBuilder(&recordingRuntime{available: true})

	builder.SetBuildTimeout(0)
	builder.SetPublishTimeout(-time.Minute)
	if builder.buildTimeout != baseImageBuildTimeout || builder.publishTimeout != baseImagePublishTimeout {
		t.Fatalf("defaults were overwritten: build=%s publish=%s",
			builder.buildTimeout, builder.publishTimeout)
	}

	builder.SetBuildTimeout(90 * time.Minute)
	builder.SetPublishTimeout(30 * time.Minute)
	if builder.buildTimeout != 90*time.Minute || builder.publishTimeout != 30*time.Minute {
		t.Fatalf("overrides not applied: build=%s publish=%s",
			builder.buildTimeout, builder.publishTimeout)
	}
}

func containsEvent(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}
