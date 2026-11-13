package browser

import (
	_ "embed"
	"regexp"
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
		"__PW_CFT_VERSION__":               "PW_CFT_VERSION",
		"__PW_VENDOR_REPO__":               "PW_VENDOR_REPO",
		"__PW_VENDOR_RELEASE_TAG__":        "PW_VENDOR_RELEASE_TAG",
		"__PW_CHROME_LINUX64_SHA256__":     "PW_CHROME_LINUX64_SHA256",
		"__PW_CHROME_LINUX_ARM64_SHA256__": "PW_CHROME_LINUX_ARM64_SHA256",
		"__PW_FFMPEG_LINUX_ARM64_SHA256__": "PW_FFMPEG_LINUX_ARM64_SHA256",
		"__PW_FFMPEG_BUILD__":              "PW_FFMPEG_BUILD",
		"__PW_FFMPEG_LINUX_SHA256__":       "PW_FFMPEG_LINUX_SHA256",
	} {
		script = strings.ReplaceAll(script, placeholder, r.pin(key))
	}
	return strings.ReplaceAll(r.render(script), "__BROWSER_STACK_CHECK__", r.stackCheck())
}

func (r browserAssetRenderer) browserDir() string {
	return "/root/.cache/ms-playwright/remote-chrome-" + r.pin("PW_CFT_VERSION")
}

func (r browserAssetRenderer) render(script string) string {
	return strings.NewReplacer(
		"__BROWSER_DIR__", r.browserDir(),
		"__PW_CFT_VERSION__", r.pin("PW_CFT_VERSION"),
		"__CHROME_VERSION_PATTERN__", r.versionPattern(),
	).Replace(script)
}

func (r browserAssetRenderer) launcherScript() []byte {
	return []byte(r.render(embeddedGUIUpScript))
}

func (r browserAssetRenderer) versionPattern() string {
	// Both architectures use the same fully patched CfT version. Accepting
	// an arbitrary ARM patch would also accept builds predating security fixes.
	return `^(Google Chrome for Testing|Chromium) ` + regexp.QuoteMeta(r.pin("PW_CFT_VERSION")) + `[[:space:]]*$`
}

func (r browserAssetRenderer) stackCheck() string {
	return `command -v Xvfb >/dev/null 2>&1 && ` + r.browserDir() + `/chrome --version 2>/dev/null | grep -Eq '` + r.versionPattern() + `'`
}

func renderedGUIUpScript() []byte {
	return newBrowserAssetRenderer().launcherScript()
}

func browserStackCheck() string {
	return newBrowserAssetRenderer().stackCheck()
}
