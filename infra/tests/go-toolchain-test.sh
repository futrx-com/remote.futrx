#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../versions.env
. "$SCRIPT_DIR/../versions.env"
# shellcheck source=../lib/go-toolchain.sh
. "$SCRIPT_DIR/../lib/go-toolchain.sh"

log()  { :; }
warn() { :; }
ok()   { :; }
ERROR_LOG=""
err()  { ERROR_LOG="${ERROR_LOG}${*}"$'\n'; }

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

TEST_ROOT="$(command mktemp -d "${TMPDIR:-/tmp}/remote-go-toolchain-test.XXXXXX")"
trap 'command rm -rf "$TEST_ROOT"' EXIT

# Scenarios must not see a Go the host happens to have on PATH: a CI runner
# whose system Go matches the pin would turn a fresh install into a no-op.
# Expose only the utilities the installer and this harness actually use.
TEST_SYSTEM_BIN="$TEST_ROOT/system-bin"
command mkdir -p "$TEST_SYSTEM_BIN"
for tool in chmod grep head ln mkdir mktemp mv rm rmdir sed; do
    tool_path="$(command -v "$tool")" || fail "missing test dependency: $tool"
    command ln -s "$tool_path" "$TEST_SYSTEM_BIN/$tool"
done

TEST_ARCH=amd64
CURL_CALLS=0
FAIL_DOWNLOADS_BEFORE_SUCCESS=0
FAIL_ALL_DOWNLOADS=0
TEST_GO_VERSION="$GO_VERSION"
OLD_GO_VERSION=0.0.1
FAKE_GO_VERSION="$TEST_GO_VERSION"
FAKE_ARCHIVE_LAYOUT=official
FAIL_GO_INSTALL_MOVE=0
FAIL_GO_RESTORE_MOVE=0
LAST_GO_BACKUP=""

assert_eq() {
    local expected="$1" actual="$2" label="$3"
    if [ "$expected" != "$actual" ]; then
        fail "$label: expected '$expected', got '$actual'"
    fi
}

# Production runs on GNU/Linux, where mktemp supports --suffix. This small
# adapter keeps the deterministic fixture test runnable on both Linux and macOS.
mktemp() {
    if [ "${1:-}" = "--suffix=.tgz" ]; then
        command mktemp "${TMPDIR:-/tmp}/remote-go-archive.XXXXXX"
        return
    fi
    command mktemp "$@"
}

mv() {
    local source="${1:-}" destination="${2:-}"
    case "$source" in
        "$GO_INSTALL_ROOT"/.go-backup.*/go)
            LAST_GO_BACKUP="$source"
            [ "$FAIL_GO_RESTORE_MOVE" -eq 0 ] || return 1
            ;;
        */ready)
            if [ "$destination" = "$GO_INSTALL_ROOT/go" ]; then
                [ "$FAIL_GO_INSTALL_MOVE" -eq 0 ] || return 1
            fi
            ;;
    esac
    command mv "$@"
}

dpkg() {
    case "${1:-}" in
        --print-architecture) printf '%s\n' "$TEST_ARCH" ;;
        -s) return 1 ;;
        *) return 1 ;;
    esac
}

fetch_go_toolchain_github_url() {
    printf '%s\n' 'https://github.test/actions/go-versions/go-linux.tar.gz'
}

write_fake_go_tree() {
    local root="$1" version="$2"
    mkdir -p "$root/bin"
    printf '%s\n' '#!/bin/sh' "echo 'go version go${version} linux/amd64'" > "$root/bin/go"
    printf '%s\n' '#!/bin/sh' 'exit 0' > "$root/bin/gofmt"
    chmod +x "$root/bin/go" "$root/bin/gofmt"
}

# The install function validates and extracts with tar. Tests replace the
# network archive with a tiny deterministic fixture containing fake binaries.
tar() {
    if [ "${1:-}" = "-tzf" ]; then
        return 0
    fi
    local destination=""
    while [ "$#" -gt 0 ]; do
        case "$1" in
            -C) destination="$2"; shift 2 ;;
            *) shift ;;
        esac
    done
    [ -n "$destination" ] || return 1
    if [ "$FAKE_ARCHIVE_LAYOUT" = "github" ]; then
        write_fake_go_tree "$destination" "$FAKE_GO_VERSION"
    else
        write_fake_go_tree "$destination/go" "$FAKE_GO_VERSION"
    fi
}

curl() {
    CURL_CALLS=$((CURL_CALLS + 1))
    local output="" url=""
    while [ "$#" -gt 0 ]; do
        case "$1" in
            -o) output="$2"; shift 2 ;;
            *) url="$1"; shift ;;
        esac
    done
    if [ "$FAIL_ALL_DOWNLOADS" -eq 1 ] \
        || [ "$CURL_CALLS" -le "$FAIL_DOWNLOADS_BEFORE_SUCCESS" ]; then
        return 22
    fi
    [ -n "$output" ] || return 1
    if [[ "$url" == https://github.test/* ]]; then
        FAKE_ARCHIVE_LAYOUT=github
    else
        FAKE_ARCHIVE_LAYOUT=official
    fi
    printf 'fixture archive\n' > "$output"
}

use_install_root() {
    local name="$1"
    GO_INSTALL_ROOT="$TEST_ROOT/$name/usr/local"
    export GO_INSTALL_ROOT
    mkdir -p "$GO_INSTALL_ROOT/bin"
    PATH="$GO_INSTALL_ROOT/bin:$TEST_SYSTEM_BIN"
    export PATH
    hash -r
}

stage_fake_go_toolchain() {
    local version="$1"
    STAGED_GO_TOOLCHAIN="$GO_INSTALL_ROOT/.test-stage"
    mkdir -p "$STAGED_GO_TOOLCHAIN"
    write_fake_go_tree "$STAGED_GO_TOOLCHAIN/ready" "$version"
}

# Debian-to-Go architecture aliases.
assert_eq amd64 "$(go_toolchain_arch amd64)" "amd64 mapping"
assert_eq arm64 "$(go_toolchain_arch arm64)" "arm64 mapping"
assert_eq 386 "$(go_toolchain_arch i386)" "i386 mapping"
assert_eq armv6l "$(go_toolchain_arch armhf)" "armhf mapping"
assert_eq ppc64le "$(go_toolchain_arch ppc64el)" "ppc64el mapping"

# Fresh server: no existing Go installation.
use_install_root fresh
CURL_CALLS=0
FAIL_DOWNLOADS_BEFORE_SUCCESS=0
FAIL_ALL_DOWNLOADS=0
ensure_go_toolchain "$TEST_GO_VERSION"
assert_eq "$TEST_GO_VERSION" "$(go_toolchain_version "$GO_INSTALL_ROOT/bin/go")" "fresh install"
assert_eq 1 "$CURL_CALLS" "fresh download count"

# Idempotent rerun: matching Go must not touch the network.
CURL_CALLS=0
FAIL_ALL_DOWNLOADS=1
ensure_go_toolchain "$TEST_GO_VERSION"
assert_eq 0 "$CURL_CALLS" "idempotent rerun download count"

# Existing/old server: shadow an older distro Go with the exact pin.
use_install_root distro-upgrade
DISTRO_GO_ROOT="$TEST_ROOT/distro-upgrade/usr"
write_fake_go_tree "$DISTRO_GO_ROOT" "$OLD_GO_VERSION"
PATH="$GO_INSTALL_ROOT/bin:$DISTRO_GO_ROOT/bin:$TEST_SYSTEM_BIN"
export PATH
hash -r
CURL_CALLS=0
FAIL_ALL_DOWNLOADS=0
ensure_go_toolchain "$TEST_GO_VERSION"
assert_eq "$TEST_GO_VERSION" "$(go_toolchain_version "$GO_INSTALL_ROOT/bin/go")" "distro Go upgrade"

# Existing pinned/local install: replace it without an unsafe in-place extract.
use_install_root local-upgrade
write_fake_go_tree "$GO_INSTALL_ROOT/go" "$OLD_GO_VERSION"
ln -sf "$GO_INSTALL_ROOT/go/bin/go" "$GO_INSTALL_ROOT/bin/go"
ln -sf "$GO_INSTALL_ROOT/go/bin/gofmt" "$GO_INSTALL_ROOT/bin/gofmt"
hash -r
CURL_CALLS=0
ensure_go_toolchain "$TEST_GO_VERSION"
assert_eq "$TEST_GO_VERSION" "$(go_toolchain_version "$GO_INSTALL_ROOT/bin/go")" "local Go upgrade"

# Both Google-backed endpoints fail: automatically use the GitHub archive.
use_install_root fallback
CURL_CALLS=0
FAIL_DOWNLOADS_BEFORE_SUCCESS=2
FAIL_ALL_DOWNLOADS=0
ensure_go_toolchain "$TEST_GO_VERSION"
assert_eq "$TEST_GO_VERSION" "$(go_toolchain_version "$GO_INSTALL_ROOT/bin/go")" "fallback install"
assert_eq 3 "$CURL_CALLS" "fallback download count"

# Total download failure must preserve the old working toolchain.
use_install_root preserve
write_fake_go_tree "$GO_INSTALL_ROOT/go" "$OLD_GO_VERSION"
ln -sf "$GO_INSTALL_ROOT/go/bin/go" "$GO_INSTALL_ROOT/bin/go"
ln -sf "$GO_INSTALL_ROOT/go/bin/gofmt" "$GO_INSTALL_ROOT/bin/gofmt"
hash -r
CURL_CALLS=0
FAIL_DOWNLOADS_BEFORE_SUCCESS=0
FAIL_ALL_DOWNLOADS=1
if ensure_go_toolchain "$TEST_GO_VERSION"; then
    fail "download failure unexpectedly succeeded"
fi
assert_eq "$OLD_GO_VERSION" "$(go_toolchain_version "$GO_INSTALL_ROOT/bin/go")" "preserved old install"

# Unsupported architectures fail before touching an existing installation.
TEST_ARCH=mipsel
if ensure_go_toolchain "$TEST_GO_VERSION"; then
    fail "unsupported architecture unexpectedly succeeded"
fi
assert_eq "$OLD_GO_VERSION" "$(go_toolchain_version "$GO_INSTALL_ROOT/bin/go")" "unsupported architecture preservation"

# A failed final install move followed by a failed restore must leave the old
# toolchain in its backup and report the exact recovery location.
use_install_root failed-install-restore
write_fake_go_tree "$GO_INSTALL_ROOT/go" "$OLD_GO_VERSION"
stage_fake_go_toolchain "$TEST_GO_VERSION"
FAIL_GO_INSTALL_MOVE=1
FAIL_GO_RESTORE_MOVE=1
LAST_GO_BACKUP=""
ERROR_LOG=""
if install_staged_go_toolchain \
    "$STAGED_GO_TOOLCHAIN" "$GO_INSTALL_ROOT" "$GO_INSTALL_ROOT/go" "$TEST_GO_VERSION"; then
    fail "failed install and restore unexpectedly succeeded"
fi
[ -n "$LAST_GO_BACKUP" ] || fail "failed restore did not expose the backup path"
[ -d "$LAST_GO_BACKUP" ] || fail "failed restore deleted the Go backup"
assert_eq "$OLD_GO_VERSION" \
    "$(go_toolchain_version "$LAST_GO_BACKUP/bin/go")" \
    "backup after failed install restore"
[[ "$ERROR_LOG" == *"backup preserved at $LAST_GO_BACKUP"* ]] || \
    fail "failed install restore did not report the preserved backup path"
FAIL_GO_INSTALL_MOVE=0
FAIL_GO_RESTORE_MOVE=0

# A post-install verification failure uses the same safe restore path.
use_install_root failed-verification-restore
write_fake_go_tree "$GO_INSTALL_ROOT/go" "$OLD_GO_VERSION"
stage_fake_go_toolchain 9.9.9
FAIL_GO_RESTORE_MOVE=1
LAST_GO_BACKUP=""
ERROR_LOG=""
if install_staged_go_toolchain \
    "$STAGED_GO_TOOLCHAIN" "$GO_INSTALL_ROOT" "$GO_INSTALL_ROOT/go" "$TEST_GO_VERSION"; then
    fail "verification and restore failure unexpectedly succeeded"
fi
[ -n "$LAST_GO_BACKUP" ] || fail "verification restore failure did not expose the backup path"
[ -d "$LAST_GO_BACKUP" ] || fail "verification restore failure deleted the Go backup"
assert_eq "$OLD_GO_VERSION" \
    "$(go_toolchain_version "$LAST_GO_BACKUP/bin/go")" \
    "backup after failed verification restore"
[[ "$ERROR_LOG" == *"backup preserved at $LAST_GO_BACKUP"* ]] || \
    fail "verification restore failure did not report the preserved backup path"
FAIL_GO_RESTORE_MOVE=0

printf 'PASS: Go toolchain fresh install, upgrade, rerun, fallback, and preservation\n'
