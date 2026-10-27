#!/usr/bin/env bash
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
TEMPLATE="$TESTS_DIR/../templates/Caddyfile.tmpl"

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

site_block() {
    awk -v site="$1" '
        $0 == site " {" { in_block = 1 }
        in_block {
            print
            opens += gsub(/\{/, "{")
            closes += gsub(/\}/, "}")
            if (opens > 0 && opens == closes) exit
        }
    ' "$TEMPLATE"
}

[ -z "$(site_block 'code.${HOSTNAME}')" ] || fail 'removed shared code hostname block is still present'

for site in '*.${HOSTNAME}' '*.dev.${HOSTNAME}'; do
    block="$(site_block "$site")"
    [ -n "$block" ] || fail "$site block is missing"
    printf '%s\n' "$block" | grep -Fq 'tls {' || \
        fail "$site is not assigned an explicit TLS policy"
    printf '%s\n' "$block" | grep -Fq 'on_demand' || \
        fail "$site does not use the on-demand TLS policy"
done

echo 'Caddy template TLS policy tests passed'
