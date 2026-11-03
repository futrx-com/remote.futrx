package browser

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	containerlaunch "github.com/futrx-com/remote.futrx.com/internal/service/container/launch"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var (
	_ serviceproject.ContainerBrowser    = (*Service)(nil)
	_ provisioning.BrowserProvisioner    = (*Service)(nil)
	_ containerlaunch.BrowserProvisioner = (*Service)(nil)
)

func TestEnsureProvisionsBeforeRuntimeTransition(t *testing.T) {
	tests := []struct {
		name   string
		ensure func(*Service, context.Context, string) error
		start  string
	}{
		{name: "full stack", ensure: (*Service).Ensure, start: "start"},
		{name: "core", ensure: (*Service).EnsureCore, start: "start-core"},
		{name: "view", ensure: (*Service).EnsureView, start: "start-view"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := []string{}
			provisioner := &recordingProvisioner{events: &events}
			runtime := &recordingRuntime{events: &events}
			service := NewService(Dependencies{
				Provisioner: provisioner,
				Runtime:     runtime,
				Tooling:     stubTooling{},
			}, 6080)

			if err := tt.ensure(service, context.Background(), "c1"); err != nil {
				t.Fatalf("ensure browser: %v", err)
			}

			want := []string{"provision:c1", tt.start + ":c1"}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("events = %v, want %v", events, want)
			}
		})
	}
}

func TestEnsureStopsWhenProvisioningFails(t *testing.T) {
	wantErr := errors.New("provision failed")
	events := []string{}
	service := NewService(Dependencies{
		Provisioner: &recordingProvisioner{events: &events, err: wantErr},
		Runtime:     &recordingRuntime{events: &events},
		Tooling:     stubTooling{},
	}, 6080)

	err := service.Ensure(context.Background(), "c1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Ensure() error = %v, want %v", err, wantErr)
	}
	if want := []string{"provision:c1"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestEnsurePropagatesRuntimeFailure(t *testing.T) {
	wantErr := errors.New("start failed")
	events := []string{}
	service := NewService(Dependencies{
		Provisioner: &recordingProvisioner{events: &events},
		Runtime:     &recordingRuntime{events: &events, startErr: wantErr},
		Tooling:     stubTooling{},
	}, 6080)

	err := service.EnsureCore(context.Background(), "c1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("EnsureCore() error = %v, want %v", err, wantErr)
	}
	if want := []string{"provision:c1", "start-core:c1"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

type recordingProvisioner struct {
	events *[]string
	err    error
}

func (p *recordingProvisioner) Provision(_ context.Context, containerName string) error {
	*p.events = append(*p.events, "provision:"+containerName)
	return p.err
}

type recordingRuntime struct {
	events   *[]string
	startErr error
}

func (r *recordingRuntime) Start(_ context.Context, containerName string) error {
	*r.events = append(*r.events, "start:"+containerName)
	return r.startErr
}

func (r *recordingRuntime) StartCore(_ context.Context, containerName string) error {
	*r.events = append(*r.events, "start-core:"+containerName)
	return r.startErr
}

func (r *recordingRuntime) StartView(_ context.Context, containerName string) error {
	*r.events = append(*r.events, "start-view:"+containerName)
	return r.startErr
}

func (*recordingRuntime) Stop(context.Context, string) error { return nil }

func (*recordingRuntime) StopView(context.Context, string) error { return nil }

func (*recordingRuntime) Running(context.Context, string) (bool, error) { return false, nil }

func (*recordingRuntime) Status(context.Context, string) (serviceproject.AgentBrowserInfo, error) {
	return serviceproject.AgentBrowserInfo{}, nil
}

type stubTooling struct{}

func (stubTooling) EnsureSkill(context.Context, string) error   { return nil }
func (stubTooling) EnsureScript(context.Context, string) error  { return nil }
func (stubTooling) EnsureMCP(context.Context, string) error     { return nil }
func (stubTooling) EnsureNesting(context.Context, string) error { return nil }
