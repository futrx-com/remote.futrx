package image

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSoftwareManifestValidationAndRecipe(t *testing.T) {
	for _, input := range []string{`null`, `{"apt":["git;touch /tmp/injected"]}`, `{"apt":["--allow-unauthenticated"]}`, `{"npm":["typescript@latest"]}`, `{"script":"echo unsafe"}`, `{} {}`} {
		file := filepath.Join(t.TempDir(), "software.json")
		if err := os.WriteFile(file, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSoftware(file); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	file := filepath.Join(t.TempDir(), "software.json")
	os.WriteFile(file, []byte(`{"apt":["ripgrep","make=4.3-4.1build2"],"npm":["typescript@5.9.3"],"prebakeBrowser":false,"prebakeIDE":false}`), 0600)
	software, err := LoadSoftware(file)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{available: true}
	builder := NewBuilder(runtime, &recordingProfileSource{profiles: configuredProfiles()}, "install-browser", []byte("install-ide"), nil).WithSoftware(software)
	builder.networkWarmup = 0
	if err := builder.Build(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	events := strings.Join(runtime.events, "\n")
	if strings.Contains(events, "install-browser") || strings.Contains(events, "install-ide") || !strings.Contains(events, "'typescript@5.9.3'") || !strings.Contains(events, "software-sha256="+software.Digest) {
		t.Fatal(events)
	}
	if strings.Index(events, "'typescript@5.9.3'") > strings.Index(events, "apt-get -o DPkg::Lock::Timeout=300 clean") {
		t.Fatal("cleanup before optional software installation")
	}
}
func TestInvalidSoftwareFailsBeforeLXD(t *testing.T) {
	runtime := &recordingRuntime{available: true}
	builder := NewBuilder(runtime, nil, "", nil, nil).WithSoftware(Software{Apt: []string{"$(oops)"}})
	if err := builder.Build(context.Background(), "test"); err == nil || len(runtime.events) != 0 {
		t.Fatalf("err=%v events=%v", err, runtime.events)
	}
}
