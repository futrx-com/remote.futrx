#!/usr/bin/env bash
# Publishes the Playwright browser archives pinned in versions.env as GitHub
# release assets. They are the install-time fallback for servers whose IPs
# Google's Chrome-for-Testing CDN geo-blocks (403 "not available in your
# location" — seen on Hetzner and Scaleway ranges). See vendors/README.md.
#
# Run from any machine with clean egress (or let the "Vendor Playwright
# assets" workflow run it on a GitHub runner):
#   bash vendors/publish-playwright-assets.sh
#
# Requires: curl, node/npx, gh (authenticated with write access to the repo).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
. "$ROOT/infra/versions.env"
for v in PLAYWRIGHT_VERSION PW_CFT_VERSION PW_FFMPEG_BUILD PW_VENDOR_REPO \
         PW_VENDOR_RELEASE_TAG PW_CHROME_LINUX64_SHA256 \
         PW_CHROME_LINUX_ARM64_SHA256 PW_FFMPEG_LINUX_SHA256 PW_FFMPEG_LINUX_ARM64_SHA256; do
    [ -n "${!v:-}" ] || { echo "versions.env is missing $v" >&2; exit 1; }
done

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
    else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# Chrome deliberately need not match Playwright's bundled browser: security
# releases must ship independently. Only FFmpeg is coupled to the driver.
dryrun="$(npx --yes "playwright@${PLAYWRIGHT_VERSION}" install ffmpeg --dry-run 2>/dev/null)"
want_ffmpeg="$(printf '%s\n' "$dryrun" | sed -n 's/.*playwright ffmpeg v\([0-9]*\).*/\1/p' | head -1)"
if [ "$want_ffmpeg" != "$PW_FFMPEG_BUILD" ]; then
    echo "FFmpeg pin mismatch: driver wants ${want_ffmpeg:-?}, manifest pins $PW_FFMPEG_BUILD" >&2
    exit 1
fi

# ── Download the Linux x64 and ARM64 assets ──────────────────────────────────────────
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fetch() { # fetch <output-name> <url>...
    local out="$WORK/$1"; shift
    local url
    for url in "$@"; do
        echo "  <- $url"
        if curl -fsSL --retry 3 -o "$out" "$url"; then return 0; fi
        echo "     (failed, trying next source)"
    done
    echo "could not download $1 from any source" >&2
    return 1
}

echo "downloading assets for playwright@${PLAYWRIGHT_VERSION} (CfT ${PW_CFT_VERSION}, ffmpeg ${PW_FFMPEG_BUILD})"
fetch chrome-linux64.zip \
    "https://storage.googleapis.com/chrome-for-testing-public/${PW_CFT_VERSION}/linux64/chrome-linux64.zip"
fetch chrome-linux-arm64.zip \
    "https://storage.googleapis.com/chrome-for-testing-public/${PW_CFT_VERSION}/linux-arm64/chrome-linux-arm64.zip"
fetch ffmpeg-linux.zip \
    "https://cdn.playwright.dev/dbazure/download/playwright/builds/ffmpeg/${PW_FFMPEG_BUILD}/ffmpeg-linux.zip" \
    "https://playwright.download.prss.microsoft.com/dbazure/download/playwright/builds/ffmpeg/${PW_FFMPEG_BUILD}/ffmpeg-linux.zip"
fetch ffmpeg-linux-arm64.zip \
    "https://cdn.playwright.dev/dbazure/download/playwright/builds/ffmpeg/${PW_FFMPEG_BUILD}/ffmpeg-linux-arm64.zip"

# ── Verify against the committed pins ────────────────────────────────────────
fail=0
check() { # check <file> <pinned-sha>
    local got; got="$(sha256 "$WORK/$1")"
    if [ "$got" != "$2" ]; then
        echo "sha256 mismatch for $1:" >&2
        echo "  pinned: $2" >&2
        echo "  actual: $got" >&2
        fail=1
    fi
}
check chrome-linux64.zip "$PW_CHROME_LINUX64_SHA256"
check chrome-linux-arm64.zip "$PW_CHROME_LINUX_ARM64_SHA256"
check ffmpeg-linux.zip "$PW_FFMPEG_LINUX_SHA256"
check ffmpeg-linux-arm64.zip "$PW_FFMPEG_LINUX_ARM64_SHA256"
if [ "$fail" -ne 0 ]; then
    echo "If you just bumped PLAYWRIGHT_VERSION, update the PW_*_SHA256 pins in" >&2
    echo "versions.env to the 'actual' values above and re-run." >&2
    exit 1
fi
echo "sha256 pins verified"

# ── Publish (create-or-update; the tag itself is never moved) ────────────────
if ! gh release view "$PW_VENDOR_RELEASE_TAG" -R "$PW_VENDOR_REPO" >/dev/null 2>&1; then
    # --prerelease keeps asset plumbing off the "Latest release" slot; the
    # Releases page belongs to version tags (see release-on-tag.yml).
    gh release create "$PW_VENDOR_RELEASE_TAG" -R "$PW_VENDOR_REPO" --prerelease --target "$(git rev-parse HEAD)" \
        --title "Vendored Playwright assets (playwright@${PLAYWRIGHT_VERSION})" \
        --notes "Unmodified Playwright/Chrome-for-Testing archives (CfT ${PW_CFT_VERSION}, ffmpeg ${PW_FFMPEG_BUILD}), republished as the install-time fallback for servers geo-blocked by Google's CDN. sha256 pins live in versions.env; provenance is this repo's 'Vendor Playwright assets' workflow. See vendors/README.md."
fi
gh release upload "$PW_VENDOR_RELEASE_TAG" -R "$PW_VENDOR_REPO" --clobber \
    "$WORK/chrome-linux64.zip" \
    "$WORK/chrome-linux-arm64.zip" \
    "$WORK/ffmpeg-linux.zip" \
    "$WORK/ffmpeg-linux-arm64.zip"

# ── Round-trip: the URLs the install fallback will use must serve the bytes ──
base="https://github.com/${PW_VENDOR_REPO}/releases/download/${PW_VENDOR_RELEASE_TAG}"
for f in chrome-linux64.zip chrome-linux-arm64.zip ffmpeg-linux.zip ffmpeg-linux-arm64.zip; do
    curl -fsSL --retry 3 -o "$WORK/rt-$f" "$base/$f"
    [ "$(sha256 "$WORK/rt-$f")" = "$(sha256 "$WORK/$f")" ] \
        || { echo "round-trip sha mismatch for $f" >&2; exit 1; }
done
echo "published and verified: $base"
