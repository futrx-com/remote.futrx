#!/usr/bin/env bash
set -euo pipefail

INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$INFRA_DIR/lib/caddy-package.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
printf 'verified Caddy package\n' > "$TEST_DIR/package.deb"
valid_checksum="$(sha512sum "$TEST_DIR/package.deb" | awk '{print $1}')"

dpkg() { printf '%s\n' "$test_arch"; }
dpkg-deb() {
    case "$3" in
        Package) [ "$scenario" != wrong-package ] && echo caddy || echo other ;;
        Version) [ "$scenario" != wrong-version ] && echo 2.11.7 || echo 2.11.6 ;;
        Architecture) [ "$scenario" != wrong-architecture ] && echo "$test_arch" || echo other ;;
        *) return 1 ;;
    esac
}
curl() {
    local argument output='' url=''
    while [ "$#" -gt 0 ]; do
        argument="$1"; shift
        case "$argument" in
            -o) output="$1"; shift ;;
            https://*) url="$argument" ;;
        esac
    done
    echo "$url" >> "$TEST_DIR/downloads"
    [ "$scenario" != download-failure ] || return 22
    if [[ "$url" == *'/releases/latest' ]]; then
        [ "$scenario" != invalid-release ] && echo '{"tag_name":"v2.11.7"}' || echo '{"tag_name":"v2.11.7-beta.1"}'
    elif [[ "$url" == *'_checksums.txt' ]]; then
        if [ "$scenario" = missing-checksum ]; then
            : > "$output"
        else
            printf '%s  caddy_2.11.7_linux_%s.deb\n' "$valid_checksum" "$release_arch" > "$output"
            if [ "$scenario" = duplicate-checksum ]; then
                cat "$output" >> "$output.copy"
                cat "$output.copy" >> "$output"
            fi
        fi
    elif [[ "$url" == *'.deb' ]]; then
        if [ "$scenario" = corrupt-package ]; then echo corrupt > "$output"
        else cp "$TEST_DIR/package.deb" "$output"; fi
    else return 1
    fi
}
host_apt_get() {
    printf '%s\n' "$*" > "$TEST_DIR/install"
    [ "$scenario" != install-failure ] || return 100
    [ -s "${@: -1}" ] || return 1
}
caddy() {
    [ "$scenario" != installed-version-mismatch ] && echo 'v2.11.7 hash' || echo 'v2.11.6 hash'
}

for test_arch in amd64 arm64 armhf armel ppc64el riscv64 s390x; do
    scenario=success
    case "$test_arch" in
        armhf) release_arch=armv7 ;;
        armel) release_arch=armv5 ;;
        ppc64el) release_arch=ppc64le ;;
        *) release_arch="$test_arch" ;;
    esac
    : > "$TEST_DIR/downloads"
    rm -f "$TEST_DIR/install"
    install_caddy_from_github > "$TEST_DIR/out" 2>&1 || fail "fresh $test_arch install"
    grep -Fq "caddy_2.11.7_linux_${release_arch}.deb" "$TEST_DIR/install" || fail 'wrong architecture installed'
    grep -Fq 'https://github.com/caddyserver/caddy/releases/download/v2.11.7/' "$TEST_DIR/downloads" || fail 'unexpected download source'
    package_path="$(awk '{print $NF}' "$TEST_DIR/install")"
    [ ! -e "$package_path" ] || fail 'downloaded package survived cleanup'
done

test_arch=amd64 release_arch=amd64
for scenario in invalid-release download-failure missing-checksum duplicate-checksum corrupt-package wrong-package wrong-version wrong-architecture install-failure installed-version-mismatch; do
    rm -f "$TEST_DIR/install"
    : > "$TEST_DIR/downloads"
    if install_caddy_from_github > "$TEST_DIR/out" 2>&1; then fail "accepted $scenario"; fi
    if [ "$scenario" != installed-version-mismatch ] && [ "$scenario" != install-failure ]; then
        [ ! -e "$TEST_DIR/install" ] || fail "$scenario installed an unverified package"
    fi
done

test_arch=unsupported scenario=success
: > "$TEST_DIR/downloads"
if install_caddy_from_github > "$TEST_DIR/out" 2>&1; then fail 'unsupported architecture accepted'; fi
[ ! -s "$TEST_DIR/downloads" ] || fail 'unsupported architecture downloaded assets'

echo 'PASS: caddy-package'
