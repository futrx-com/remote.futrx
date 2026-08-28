package browser

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

func TestParseAgentBrowserStatus(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want serviceproject.AgentBrowserInfo
	}{
		{
			name: "empty output is stopped",
			want: serviceproject.AgentBrowserInfo{
				Status: serviceproject.AgentBrowserStatusStopped,
				Core:   "off",
				View:   "off",
			},
		},
		{
			name: "core and view ready",
			out:  "core=ready view=ready clients=3 uptime_sec=42",
			want: serviceproject.AgentBrowserInfo{
				Status:      serviceproject.AgentBrowserStatusReady,
				Core:        "ready",
				View:        "ready",
				ViewerCount: 3,
				UptimeSec:   42,
			},
		},
		{
			name: "core ready without view",
			out:  "view=off core=ready",
			want: serviceproject.AgentBrowserInfo{
				Status: serviceproject.AgentBrowserStatusCoreReady,
				Core:   "ready",
				View:   "off",
			},
		},
		{
			name: "view alone remains stopped and invalid counts are ignored",
			out:  "core=off view=ready clients=many uptime_sec=unknown ignored=value",
			want: serviceproject.AgentBrowserInfo{
				Status: serviceproject.AgentBrowserStatusStopped,
				Core:   "off",
				View:   "ready",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseAgentBrowserStatus(tt.out); got != tt.want {
				t.Fatalf("parseAgentBrowserStatus(%q) = %#v, want %#v", tt.out, got, tt.want)
			}
		})
	}
}

func TestAdapterProvisionPublishesRuntimeAssets(t *testing.T) {
	runner := &browserRecordingRunner{}
	adapter := NewAdapter(runner, nil, assets.NewPublisher(runner))

	if err := adapter.Provision(context.Background(), "c1"); err != nil {
		t.Fatalf("provision browser: %v", err)
	}

	want := []string{
		"exec c1 -- sh -c " + browserStackCheck(),
		"exec c1 -- install -d -m 755 " + containerGUIDir,
		"exec c1 -- cat " + containerGUIScriptHash,
		"exec c1 -- cat " + containerHumanInputHash,
	}
	if got := strings.Join(runner.calls, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("command order:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
}

func TestAdapterStartsExpectedRuntimeVerbWithoutProvisioning(t *testing.T) {
	tests := []struct {
		name  string
		start func(*Adapter, context.Context, string) error
		verb  string
	}{
		{name: "full stack", start: (*Adapter).Start, verb: "start"},
		{name: "core", start: (*Adapter).StartCore, verb: "start-core"},
		{name: "view", start: (*Adapter).StartView, verb: "start-view"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &browserRecordingRunner{}
			adapter := NewAdapter(runner, nil, assets.NewPublisher(runner))

			if err := tt.start(adapter, context.Background(), "c1"); err != nil {
				t.Fatalf("start browser: %v", err)
			}

			want := []string{"exec c1 -- sh " + containerGUIScript + " " + tt.verb}
			if got := strings.Join(runner.calls, "\n"); got != strings.Join(want, "\n") {
				t.Fatalf("command order:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
			}
		})
	}
}

type browserRecordingRunner struct {
	calls []string
}

func (r *browserRecordingRunner) Available() bool { return true }

func (r *browserRecordingRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	switch args[len(args)-1] {
	case containerGUIScriptHash:
		return assets.Hash(renderedGUIUpScript()), nil
	case containerHumanInputHash:
		return assets.Hash(humanInputScript), nil
	default:
		return "", nil
	}
}

func TestRenderedGUIUpScriptPinsAndValidatesChromium(t *testing.T) {
	script := string(renderedGUIUpScript())
	for _, want := range []string{
		"EXPECTED_CHROME_VERSION=" + provisioning.MustPin("PW_CFT_VERSION"),
		"chromium-*/chrome-linux64/chrome",
		"chromium-*/chrome-linux/chrome",
		"clear_stale_profile_locks",
		"browser core exited before CDP became ready",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("rendered GUI launcher is missing %q", want)
		}
	}
	if strings.Contains(script, "__PW_CFT_VERSION__") {
		t.Fatal("rendered GUI launcher still contains the Chrome version placeholder")
	}
	stackCheck := browserStackCheck()
	for _, want := range []string{"chromium-*/chrome-linux64/chrome", "chromium-*/chrome-linux/chrome"} {
		if !strings.Contains(stackCheck, want) {
			t.Fatalf("browser stack check is missing %q", want)
		}
	}
	if strings.Contains(script, "--enable-unsafe-swiftshader") {
		t.Fatal("rendered GUI launcher opts in to unsafe SwiftShader WebGL")
	}
}

func (r *browserRecordingRunner) RunStdin(context.Context, io.Reader, ...string) (string, error) {
	return "", nil
}
