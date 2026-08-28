package browser

import (
	_ "embed"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

//go:embed assets/agent-browser-install.sh
var embeddedAgentBrowserInstallScript string

//go:embed assets/agent-browser-smoke-test.sh
var embeddedAgentBrowserSmokeTestScript string

//go:embed assets/gui-up.sh
var embeddedGUIUpScript string

type browserAssetRenderer struct {
	pin func(string) string
}

func newBrowserAssetRenderer() browserAssetRenderer {
	return browserAssetRenderer{pin: provisioning.MustPin}
}

// InstallScript installs the headed-browser stack used by the Agent Browser
// feature. It is shared by base-image builds and the on-demand repair path
// for older containers. The __-delimited pins in the embedded asset are
// filled from versions.env so the Playwright version and the sha256-gated
// vendor fallback stay declared in one place.
func InstallScript() string {
	return newBrowserAssetRenderer().installScript()
}

func (r browserAssetRenderer) installScript() string {
	script := strings.TrimSuffix(embeddedAgentBrowserInstallScript, "\n") + "\n\n" +
		strings.TrimSuffix(embeddedAgentBrowserSmokeTestScript, "\n")
	for placeholder, key := range map[string]string{
		"__PLAYWRIGHT_VERSION__":                   "PLAYWRIGHT_VERSION",
		"__PW_CFT_VERSION__":                       "PW_CFT_VERSION",
		"__PW_VENDOR_REPO__":                       "PW_VENDOR_REPO",
		"__PW_VENDOR_RELEASE_TAG__":                "PW_VENDOR_RELEASE_TAG",
		"__PW_CHROME_LINUX64_SHA256__":             "PW_CHROME_LINUX64_SHA256",
		"__PW_HEADLESS_SHELL_LINUX64_SHA256__":     "PW_HEADLESS_SHELL_LINUX64_SHA256",
		"__PW_FFMPEG_LINUX_SHA256__":               "PW_FFMPEG_LINUX_SHA256",
		"__PW_CHROMIUM_LINUX_ARM64_SHA256__":       "PW_CHROMIUM_LINUX_ARM64_SHA256",
		"__PW_HEADLESS_SHELL_LINUX_ARM64_SHA256__": "PW_HEADLESS_SHELL_LINUX_ARM64_SHA256",
		"__PW_FFMPEG_LINUX_ARM64_SHA256__":         "PW_FFMPEG_LINUX_ARM64_SHA256",
	} {
		script = strings.ReplaceAll(script, placeholder, r.pin(key))
	}
	return script
}

func (r browserAssetRenderer) launcherScript() []byte {
	return []byte(strings.ReplaceAll(
		embeddedGUIUpScript,
		"__PW_CFT_VERSION__",
		r.pin("PW_CFT_VERSION"),
	))
}

func (r browserAssetRenderer) stackCheck() string {
	return `command -v Xvfb >/dev/null 2>&1 && for browser_bin in /root/.cache/ms-playwright/chromium-*/chrome-linux64/chrome /root/.cache/ms-playwright/chromium-*/chrome-linux/chrome; do [ -x "$browser_bin" ] || continue; "$browser_bin" --version 2>/dev/null | grep -Fq '` + r.pin("PW_CFT_VERSION") + `' && exit 0; done; exit 1`
}

func renderedGUIUpScript() []byte {
	return newBrowserAssetRenderer().launcherScript()
}

func browserStackCheck() string {
	return newBrowserAssetRenderer().stackCheck()
}
