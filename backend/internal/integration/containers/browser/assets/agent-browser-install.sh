set -e
# Agent preparation and the Browser pane can request repair concurrently.
mkdir -p /opt/remote-browser
exec 9>/opt/remote-browser/.install.lock
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

# Keep the browser independent of Playwright's release cadence. Install a
# versioned runtime atomically, leaving any running older binary intact until
# the launcher restarts it. Never fall back to an old workspace browser cache.
PLAYWRIGHT_VERSION=__PLAYWRIGHT_VERSION__
PW_CFT_VERSION=__PW_CFT_VERSION__
PW_FFMPEG_BUILD=__PW_FFMPEG_BUILD__
BROWSER_RUNTIME=__BROWSER_RUNTIME_DIR__
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

mkdir -p /opt/remote-browser
BROWSER_STAGE="$(mktemp -d /opt/remote-browser/.install.XXXXXX)"
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
mv "$BROWSER_STAGE/$CHROME_ARCHIVE" "$BROWSER_STAGE/chrome"
mkdir -p "$BROWSER_STAGE/browsers/ffmpeg-$PW_FFMPEG_BUILD"
unzip -q "$BROWSER_STAGE/$FFMPEG_ARCHIVE.zip" -d "$BROWSER_STAGE/browsers/ffmpeg-$PW_FFMPEG_BUILD"
chmod 755 "$BROWSER_STAGE/chrome/chrome" "$BROWSER_STAGE/browsers/ffmpeg-$PW_FFMPEG_BUILD/ffmpeg-linux"
"$BROWSER_STAGE/chrome/chrome" --version | grep -Eq '__CHROME_VERSION_PATTERN__'
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm install --prefix "$BROWSER_STAGE" \
    --ignore-scripts --no-audit --no-fund --save-exact "playwright@$PLAYWRIGHT_VERSION"
rm -f "$BROWSER_STAGE/$CHROME_ARCHIVE.zip" "$BROWSER_STAGE/$FFMPEG_ARCHIVE.zip"
# A failed earlier install may have left this exact runtime incomplete.
rm -rf -- "$BROWSER_RUNTIME"
mv "$BROWSER_STAGE" "$BROWSER_RUNTIME"
trap - EXIT
