package browser

import (
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

func TestInstallScriptSubstitutesEveryPin(t *testing.T) {
	script := InstallScript()
	if strings.Contains(script, "__PW_") || strings.Contains(script, "__PLAYWRIGHT_") {
		t.Fatalf("install script still contains unsubstituted placeholders:\n%s", script)
	}
	for _, want := range []string{
		"npx --yes \"playwright@${PLAYWRIGHT_VERSION}\"",
		"PW_CFT_VERSION=" + provisioning.MustPin("PW_CFT_VERSION"),
		"chromium-*/chrome-linux64/chrome",
		"chromium-*/chrome-linux/chrome",
		"was not found or executable for $(dpkg --print-architecture)",
		"amd64)",
		"arm64)",
		"chrome-linux64.zip",
		"chromium-linux-arm64.zip",
		provisioning.MustPin("PW_CHROMIUM_LINUX_ARM64_SHA256"),
		"releases/download/",
		"sha256sum -c",
		"http://127.0.0.1:19222/json/version",
		"Chrome exited after initially opening CDP",
		"setsid \"$CHROME\"",
		"kill -TERM -- \"-$SMOKE_CHROME_PID\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install script is missing %q", want)
		}
	}
	if strings.Contains(script, "--enable-unsafe-swiftshader") {
		t.Fatal("install script opts in to unsafe SwiftShader WebGL")
	}
}
