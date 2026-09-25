package applications

import (
	"strings"
	"testing"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

func TestContainerBuildScriptBuildsCommandsAndUsesMarker(t *testing.T) {
	script := string(containerBuildScript("example", "2+abc", []string{"agent", "worker"}))
	for _, want := range []string{
		"/usr/local/lib/remote/example.build",
		"./cmd/agent",
		"./cmd/worker",
		"/usr/local/bin/agent",
		"/usr/local/bin/worker",
		"go" + configconstants.ApplicationContainerGoVersion,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("generated script does not contain %q", want)
		}
	}
	if strings.Contains(script, `agent --version`) || strings.Contains(script, `worker --version`) {
		t.Fatal("generated script requires a application-defined --version command")
	}
}

func TestContainerBuildScriptBuildsRootProgramAsApplicationID(t *testing.T) {
	script := string(containerBuildScript("example", "1+abc", nil))
	if !strings.Contains(script, `-o "$APP_BUILD_DIR"/'example' '.'`) {
		t.Fatalf("root build command missing from:\n%s", script)
	}
}

func TestContainerBuildScriptDoesNotConfuseMatchingCommandWithRootProgram(t *testing.T) {
	script := string(containerBuildScript("s3disk", "1+abc", []string{"s3disk"}))
	if !strings.Contains(script, "./cmd/s3disk") {
		t.Fatalf("named command build target missing from:\n%s", script)
	}
}

func TestContainerBuildScriptQuotesCommandPaths(t *testing.T) {
	command := "worker's tool"
	script := string(containerBuildScript("example", "1+abc", []string{command}))
	for _, want := range []string{
		shellQuote("./cmd/" + command),
		shellQuote("/usr/local/bin/" + command),
	} {
		if !strings.Contains(script, want) {
			t.Errorf("generated script does not contain quoted path %q", want)
		}
	}
}
