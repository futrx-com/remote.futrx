package provisioning

import (
	"strings"
	"testing"
)

func TestInstructionsTemplateUsesInstalledHostname(t *testing.T) {
	content := string(InstructionsTemplate("Remote.Example.com."))

	for _, want := range []string{
		"https://remote.example.com",
		"https://<this-project-slug>--<port>.dev.remote.example.com",
		"`.dev.remote.example.com`",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rendered instructions do not contain %q", want)
		}
	}
	if strings.Contains(content, publicHostnamePlaceholder) {
		t.Fatal("rendered instructions still contain the hostname placeholder")
	}
	if strings.Contains(content, "remote.futrx.com") {
		t.Fatal("rendered instructions contain the production hostname")
	}
}
