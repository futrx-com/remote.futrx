set -e
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
    dbus-x11 fonts-liberation fonts-noto-core fonts-noto-color-emoji

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

# Browser: Playwright's Chromium (Chrome for Testing) — its install path
# (/root/.cache/ms-playwright) matches no AppArmor profile attachment, which
# is why it always networked while google-chrome-stable did not, and it is
# where gui-up.sh and browser.mjs both look. google-chrome-stable also works
# now thanks to the local include above; the Playwright build stays the
# baked-in default.
#
# __-delimited pins below are substituted from versions.env by
# backend/internal/integration/containers/browser/install.go.
PLAYWRIGHT_VERSION=__PLAYWRIGHT_VERSION__
PW_CFT_VERSION=__PW_CFT_VERSION__
VENDOR_URL="https://github.com/__PW_VENDOR_REPO__/releases/download/__PW_VENDOR_RELEASE_TAG__"

pw_install() {
    # pipefail so the tail filter cannot mask a failed install — a masked
    # failure here once surfaced 200 lines later as a bare "exit status 2".
    (set -o pipefail; npx --yes "playwright@${PLAYWRIGHT_VERSION}" install chromium 2>&1 | tail -20)
}

if ! pw_install; then
    # Google's Chrome-for-Testing CDN geo-blocks some datacenter IPs
    # (403 "not available in your location" — seen on Hetzner and Scaleway
    # ranges). Fall back to the project's own sha256-pinned copies of the
    # same archives, published to a GitHub release by
    # .github/workflows/vendor-playwright.yml. They are served to Playwright
    # from a loopback HTTP server so its installer (paths, revision dirs,
    # completion markers) runs untouched. See vendors/README.md.
    echo "direct Playwright download failed — retrying from vendored assets at ${VENDOR_URL}" >&2
    VENDOR_DIR=/tmp/pw-vendor
    mkdir -p "$VENDOR_DIR"
    case "$(dpkg --print-architecture)" in
        amd64)
            PW_CHROME_ARCHIVE=chrome-linux64.zip
            PW_HEADLESS_ARCHIVE=chrome-headless-shell-linux64.zip
            PW_FFMPEG_ARCHIVE=ffmpeg-linux.zip
            PW_CHROME_SHA256=__PW_CHROME_LINUX64_SHA256__
            PW_HEADLESS_SHA256=__PW_HEADLESS_SHELL_LINUX64_SHA256__
            PW_FFMPEG_SHA256=__PW_FFMPEG_LINUX_SHA256__
            ;;
        arm64)
            PW_CHROME_ARCHIVE=chromium-linux-arm64.zip
            PW_HEADLESS_ARCHIVE=chromium-headless-shell-linux-arm64.zip
            PW_FFMPEG_ARCHIVE=ffmpeg-linux-arm64.zip
            PW_CHROME_SHA256=__PW_CHROMIUM_LINUX_ARM64_SHA256__
            PW_HEADLESS_SHA256=__PW_HEADLESS_SHELL_LINUX_ARM64_SHA256__
            PW_FFMPEG_SHA256=__PW_FFMPEG_LINUX_ARM64_SHA256__
            ;;
        *)
            echo "Playwright browser fallback does not support architecture $(dpkg --print-architecture)." >&2
            exit 1
            ;;
    esac
    for f in "$PW_CHROME_ARCHIVE" "$PW_HEADLESS_ARCHIVE" "$PW_FFMPEG_ARCHIVE"; do
        curl -fsSL --retry 3 -o "$VENDOR_DIR/$f" "$VENDOR_URL/$f"
    done
    sha256sum -c --quiet <<EOF
$PW_CHROME_SHA256  $VENDOR_DIR/$PW_CHROME_ARCHIVE
$PW_HEADLESS_SHA256  $VENDOR_DIR/$PW_HEADLESS_ARCHIVE
$PW_FFMPEG_SHA256  $VENDOR_DIR/$PW_FFMPEG_ARCHIVE
EOF
    # Serve the archives by filename for whatever path Playwright requests —
    # its URL layout under a custom PLAYWRIGHT_DOWNLOAD_HOST has changed
    # between releases; the basenames have not.
    node -e '
        const http = require("http"), fs = require("fs"), path = require("path");
        const dir = process.argv[1];
        http.createServer((req, res) => {
            const name = path.basename(new URL(req.url, "http://localhost").pathname);
            const file = path.join(dir, name);
            if (!/^[A-Za-z0-9._-]+$/.test(name) || !fs.existsSync(file)) {
                res.statusCode = 404;
                return res.end("not vendored: " + name);
            }
            res.setHeader("content-length", fs.statSync(file).size);
            fs.createReadStream(file).pipe(res);
        }).listen(8377, "127.0.0.1");
    ' "$VENDOR_DIR" &
    VENDOR_SRV=$!
    trap 'kill "$VENDOR_SRV" 2>/dev/null || true' EXIT
    for _ in $(seq 1 50); do
        curl -s -o /dev/null "http://127.0.0.1:8377/" && break
        sleep 0.2
    done
    if ! PLAYWRIGHT_DOWNLOAD_HOST=http://127.0.0.1:8377 pw_install; then
        echo "Playwright install failed from both Google's CDN and the vendored release." >&2
        echo "Likely causes: this server's IP is geo-blocked by Google AND ${VENDOR_URL} is" >&2
        echo "missing assets for playwright@${PLAYWRIGHT_VERSION}. See vendors/README.md." >&2
        exit 1
    fi
    kill "$VENDOR_SRV" 2>/dev/null || true
    trap - EXIT
    rm -rf "$VENDOR_DIR"
fi
