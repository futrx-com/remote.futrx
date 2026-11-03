set -e
# Serialize concurrent Browser pane installation requests.
mkdir -p /root/.cache/ms-playwright
exec 9>/root/.cache/ms-playwright/.remote-install.lock
flock 9
if __BROWSER_STACK_CHECK__; then exit 0; fi
export DEBIAN_FRONTEND=noninteractive
# Wait for the apt/dpkg lock rather than failing if apt-daily / unattended-
# upgrades is mid-run — a common race shortly after a container boots.
APT="apt-get -o DPkg::Lock::Timeout=300"
$APT update -qq

# Virtual display + VNC bridge (x11vnc -> websockify/noVNC over HTTP/WS), a
# lightweight window manager (openbox) so the browser window keeps stable
# input focus, and xdotool to activate that window. Font packages cover
# common web / CJK / emoji glyphs so real pages render legibly.
$APT install -y -qq \
    xvfb x11vnc novnc websockify openbox xdotool \
    libgtk-3-0t64 libgbm1 libasound2t64 libnss3 libxshmfence1 \
    curl unzip dbus-x11 fonts-liberation fonts-noto-core fonts-noto-color-emoji

# Ubuntu 24.04 ships stock AppArmor profiles for browser binaries
# (/etc/apparmor.d/chrome, firefox, brave, ...) whose only purpose is to grant
# userns — but inside a nested LXD AppArmor namespace their network rules fail
# to match ("failed af match" in the host audit log), so a confined browser
# gets EVERY inet/inet6 socket create denied: no CDP socket, no page loads
# (CreatePlatformSocket: EPERM), while unconfined binaries (node, curl) work
# fine. Root-cause fix: an explicit allow-all network rule through the
# profile's local include (survives chrome package upgrades). Reload the
# profile if AppArmor is live so the on-demand install path takes effect
# immediately; at image-bake time profiles load on container boot anyway.
mkdir -p /etc/apparmor.d/local
echo "  network," > /etc/apparmor.d/local/chrome
if [ -f /etc/apparmor.d/chrome ] && command -v apparmor_parser >/dev/null 2>&1; then
    apparmor_parser -r /etc/apparmor.d/chrome 2>/dev/null || true
fi

# Install patched Chrome independently of Playwright's bundled Chromium.
# Keep the existing browser cache and workspace-provided Playwright driver.
PW_CFT_VERSION=__PW_CFT_VERSION__
PW_FFMPEG_BUILD=__PW_FFMPEG_BUILD__
BROWSER_DIR=__BROWSER_DIR__
VENDOR_URL="https://github.com/__PW_VENDOR_REPO__/releases/download/__PW_VENDOR_RELEASE_TAG__"

case "$(uname -m)" in
    x86_64)
        CFT_PLATFORM=linux64
        CHROME_ARCHIVE=chrome-linux64
        CHROME_SHA=__PW_CHROME_LINUX64_SHA256__
        FFMPEG_ARCHIVE=ffmpeg-linux
        FFMPEG_SHA=__PW_FFMPEG_LINUX_SHA256__
        ;;
    aarch64|arm64)
        CFT_PLATFORM=linux-arm64
        CHROME_ARCHIVE=chrome-linux-arm64
        CHROME_SHA=__PW_CHROME_LINUX_ARM64_SHA256__
        FFMPEG_ARCHIVE=ffmpeg-linux-arm64
        FFMPEG_SHA=__PW_FFMPEG_LINUX_ARM64_SHA256__
        ;;
    *) echo "unsupported browser architecture: $(uname -m)" >&2; exit 1 ;;
esac

BROWSER_STAGE="$(mktemp -d /root/.cache/ms-playwright/.remote-install.XXXXXX)"
trap 'rm -rf -- "$BROWSER_STAGE"' EXIT

fetch_verified() {
    local name="$1" sha="$2" url="$3"
    if ! curl -fsSL --retry 3 --connect-timeout 15 --max-time 180 -o "$BROWSER_STAGE/$name" "$url"; then
        echo "browser upstream unavailable; downloading $name from vendored assets"
        curl -fsSL --retry 3 --connect-timeout 15 --max-time 180 \
            -o "$BROWSER_STAGE/$name" "$VENDOR_URL/$name"
    fi
    printf '%s  %s\n' "$sha" "$BROWSER_STAGE/$name" | sha256sum -c --quiet
}

fetch_verified "$CHROME_ARCHIVE.zip" "$CHROME_SHA" \
    "https://storage.googleapis.com/chrome-for-testing-public/$PW_CFT_VERSION/$CFT_PLATFORM/$CHROME_ARCHIVE.zip"
fetch_verified "$FFMPEG_ARCHIVE.zip" "$FFMPEG_SHA" \
    "https://cdn.playwright.dev/dbazure/download/playwright/builds/ffmpeg/$PW_FFMPEG_BUILD/$FFMPEG_ARCHIVE.zip"

unzip -q "$BROWSER_STAGE/$CHROME_ARCHIVE.zip" -d "$BROWSER_STAGE"
"$BROWSER_STAGE/$CHROME_ARCHIVE/chrome" --version | grep -Eq '__CHROME_VERSION_PATTERN__'
mkdir -p /root/.cache/ms-playwright/ffmpeg-$PW_FFMPEG_BUILD
unzip -oq "$BROWSER_STAGE/$FFMPEG_ARCHIVE.zip" -d /root/.cache/ms-playwright/ffmpeg-$PW_FFMPEG_BUILD
chmod 755 /root/.cache/ms-playwright/ffmpeg-$PW_FFMPEG_BUILD/ffmpeg-linux
rm -rf -- "$BROWSER_DIR"
mv "$BROWSER_STAGE/$CHROME_ARCHIVE" "$BROWSER_DIR"
rm -rf -- "$BROWSER_STAGE"
trap - EXIT
