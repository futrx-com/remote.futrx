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
for v in PLAYWRIGHT_VERSION PW_CFT_VERSION PW_CHROMIUM_BUILD PW_FFMPEG_BUILD PW_VENDOR_REPO \
         PW_VENDOR_RELEASE_TAG PW_CHROME_LINUX64_SHA256 \
         PW_HEADLESS_SHELL_LINUX64_SHA256 PW_FFMPEG_LINUX_SHA256 \
         PW_CHROMIUM_LINUX_ARM64_SHA256 PW_HEADLESS_SHELL_LINUX_ARM64_SHA256 \
         PW_FFMPEG_LINUX_ARM64_SHA256; do
    [ -n "${!v:-}" ] || { echo "versions.env is missing $v" >&2; exit 1; }
done

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
    else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# ── Consistency gate ─────────────────────────────────────────────────────────
# The version/build pins must match what the pinned Playwright actually asks
# for on both supported Linux architectures, or the vendored assets would not
# match its fallback requests. Explicit platform overrides keep this gate
# independent of the machine running the publisher.
dryrun_x64="$(PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=ubuntu24.04-x64 \
    npx --yes "playwright@${PLAYWRIGHT_VERSION}" install chromium --dry-run 2>/dev/null)"
dryrun_arm64="$(PLAYWRIGHT_HOST_PLATFORM_OVERRIDE=ubuntu24.04-arm64 \
    npx --yes "playwright@${PLAYWRIGHT_VERSION}" install chromium --dry-run 2>/dev/null)"
want_cft_x64="$(printf '%s\n' "$dryrun_x64" | sed -n 's/.*Chrome for Testing \([0-9.]*\) .*/\1/p' | head -1)"
want_cft_arm64="$(printf '%s\n' "$dryrun_arm64" | sed -n 's/.*Chrome for Testing \([0-9.]*\) .*/\1/p' | head -1)"
want_chromium_x64="$(printf '%s\n' "$dryrun_x64" | sed -n 's/.*playwright chromium v\([0-9]*\).*/\1/p' | head -1)"
want_chromium_arm64="$(printf '%s\n' "$dryrun_arm64" | sed -n 's/.*playwright chromium v\([0-9]*\).*/\1/p' | head -1)"
want_ffmpeg_x64="$(printf '%s\n' "$dryrun_x64" | sed -n 's/.*playwright ffmpeg v\([0-9]*\).*/\1/p' | head -1)"
want_ffmpeg_arm64="$(printf '%s\n' "$dryrun_arm64" | sed -n 's/.*playwright ffmpeg v\([0-9]*\).*/\1/p' | head -1)"
if [ "$want_cft_x64" != "$PW_CFT_VERSION" ] \
    || [ "$want_cft_arm64" != "$PW_CFT_VERSION" ] \
    || [ "$want_chromium_x64" != "$PW_CHROMIUM_BUILD" ] \
    || [ "$want_chromium_arm64" != "$PW_CHROMIUM_BUILD" ] \
    || [ "$want_ffmpeg_x64" != "$PW_FFMPEG_BUILD" ] \
    || [ "$want_ffmpeg_arm64" != "$PW_FFMPEG_BUILD" ]; then
    echo "pin mismatch: playwright@${PLAYWRIGHT_VERSION} reports:" >&2
    echo "  amd64: CfT ${want_cft_x64:-?} / Chromium ${want_chromium_x64:-?} / ffmpeg ${want_ffmpeg_x64:-?}" >&2
    echo "  arm64: CfT ${want_cft_arm64:-?} / Chromium ${want_chromium_arm64:-?} / ffmpeg ${want_ffmpeg_arm64:-?}" >&2
    echo "versions.env pins CfT ${PW_CFT_VERSION} / Chromium ${PW_CHROMIUM_BUILD} / ffmpeg ${PW_FFMPEG_BUILD}." >&2
    echo "Update versions.env (and its sha256 pins) before publishing." >&2
    exit 1
fi

# ── Download the six Linux assets ────────────────────────────────────────────
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

ASSETS=(
    chrome-linux64.zip
    chrome-headless-shell-linux64.zip
    ffmpeg-linux.zip
    chromium-linux-arm64.zip
    chromium-headless-shell-linux-arm64.zip
    ffmpeg-linux-arm64.zip
)

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
    "https://cdn.playwright.dev/builds/cft/${PW_CFT_VERSION}/linux64/chrome-linux64.zip"
fetch chrome-headless-shell-linux64.zip \
    "https://cdn.playwright.dev/builds/cft/${PW_CFT_VERSION}/linux64/chrome-headless-shell-linux64.zip"
fetch ffmpeg-linux.zip \
    "https://cdn.playwright.dev/dbazure/download/playwright/builds/ffmpeg/${PW_FFMPEG_BUILD}/ffmpeg-linux.zip" \
    "https://playwright.download.prss.microsoft.com/dbazure/download/playwright/builds/ffmpeg/${PW_FFMPEG_BUILD}/ffmpeg-linux.zip"
fetch chromium-linux-arm64.zip \
    "https://cdn.playwright.dev/dbazure/download/playwright/builds/chromium/${PW_CHROMIUM_BUILD}/chromium-linux-arm64.zip" \
    "https://playwright.download.prss.microsoft.com/dbazure/download/playwright/builds/chromium/${PW_CHROMIUM_BUILD}/chromium-linux-arm64.zip" \
    "https://cdn.playwright.dev/builds/chromium/${PW_CHROMIUM_BUILD}/chromium-linux-arm64.zip"
fetch chromium-headless-shell-linux-arm64.zip \
    "https://cdn.playwright.dev/dbazure/download/playwright/builds/chromium/${PW_CHROMIUM_BUILD}/chromium-headless-shell-linux-arm64.zip" \
    "https://playwright.download.prss.microsoft.com/dbazure/download/playwright/builds/chromium/${PW_CHROMIUM_BUILD}/chromium-headless-shell-linux-arm64.zip" \
    "https://cdn.playwright.dev/builds/chromium/${PW_CHROMIUM_BUILD}/chromium-headless-shell-linux-arm64.zip"
fetch ffmpeg-linux-arm64.zip \
    "https://cdn.playwright.dev/dbazure/download/playwright/builds/ffmpeg/${PW_FFMPEG_BUILD}/ffmpeg-linux-arm64.zip" \
    "https://playwright.download.prss.microsoft.com/dbazure/download/playwright/builds/ffmpeg/${PW_FFMPEG_BUILD}/ffmpeg-linux-arm64.zip" \
    "https://cdn.playwright.dev/builds/ffmpeg/${PW_FFMPEG_BUILD}/ffmpeg-linux-arm64.zip"

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
check chrome-headless-shell-linux64.zip "$PW_HEADLESS_SHELL_LINUX64_SHA256"
check ffmpeg-linux.zip "$PW_FFMPEG_LINUX_SHA256"
check chromium-linux-arm64.zip "$PW_CHROMIUM_LINUX_ARM64_SHA256"
check chromium-headless-shell-linux-arm64.zip "$PW_HEADLESS_SHELL_LINUX_ARM64_SHA256"
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
    gh release create "$PW_VENDOR_RELEASE_TAG" -R "$PW_VENDOR_REPO" --prerelease \
        --title "Vendored Playwright assets (playwright@${PLAYWRIGHT_VERSION})" \
        --notes "Unmodified amd64 and arm64 Playwright/Chrome-for-Testing archives (CfT ${PW_CFT_VERSION}, Chromium ${PW_CHROMIUM_BUILD}, ffmpeg ${PW_FFMPEG_BUILD}), republished as the install-time fallback for servers geo-blocked by Google's CDN. sha256 pins live in versions.env; provenance is this repo's 'Vendor Playwright assets' workflow. See vendors/README.md."
fi
gh release upload "$PW_VENDOR_RELEASE_TAG" -R "$PW_VENDOR_REPO" --clobber \
    "${ASSETS[@]/#/$WORK/}"

# ── Round-trip: the URLs the install fallback will use must serve the bytes ──
base="https://github.com/${PW_VENDOR_REPO}/releases/download/${PW_VENDOR_RELEASE_TAG}"
for f in "${ASSETS[@]}"; do
    curl -fsSL --retry 3 -o "$WORK/rt-$f" "$base/$f"
    [ "$(sha256 "$WORK/rt-$f")" = "$(sha256 "$WORK/$f")" ] \
        || { echo "round-trip sha mismatch for $f" >&2; exit 1; }
done
echo "published and verified: $base"
