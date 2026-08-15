#!/usr/bin/env bash
set -euo pipefail

INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_TMP="$(mktemp -d)"
trap 'rm -rf "$TEST_TMP"' EXIT

export FUTRX_PLESK_ROOTS="$TEST_TMP/psa"

# shellcheck source=../lib/plesk.sh
. "$INFRA_DIR/lib/plesk.sh"
# shellcheck source=../lib/frontend-mode.sh
. "$INFRA_DIR/lib/frontend-mode.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

export HOSTNAME="remote.example.com"
export HOSTNAME_RE='remote\.example\.com'
export INSTALL_DIR="/opt/remote.futrx"
export SERVICE_PORT="7682"
export LXD_BRIDGE_IP="10.221.94.1"
export LXD_BRIDGE="lxdbr0"
export CADDY_HTTP_PORT="8080"
export PLESK_LISTEN="" PLESK_TLS="" PLESK_HTTP_SERVER=""

# The same allowlist infra/install.sh renders with. Kept in step with it by
# the assertion below that every variable the template references is listed.
ALLOWLIST='$HOSTNAME $HOSTNAME_RE $INSTALL_DIR $SERVICE_PORT $LXD_BRIDGE_IP $LXD_BRIDGE
           $CADDY_SITE_SCHEME $CADDY_SITE_PORT $CADDY_TLS_BLOCK $CADDY_GLOBAL_EXTRA
           $CADDY_HTTP_PORT $PLESK_LISTEN $PLESK_TLS $PLESK_HTTP_SERVER'

render() {
    set_caddy_render_vars "$1" "$SERVICE_PORT" "$CADDY_HTTP_PORT"
    envsubst "$ALLOWLIST" < "$INFRA_DIR/templates/Caddyfile.tmpl"
}

# ───────────────── mode selection ─────────────────

[ "$(select_frontend_mode 0)" = "standalone" ] \
    || fail "expected standalone without Plesk, got $(select_frontend_mode 0)"

mkdir -p "$TEST_TMP/psa"
printf '18.0.62 Ubuntu 24.04 1815240321.20\n' > "$TEST_TMP/psa/version"
[ "$(select_frontend_mode 0)" = "plesk" ] \
    || fail "expected plesk mode once Plesk is present"

# --no-plesk-integration is the escape hatch for an operator who has freed
# 80/443 themselves. It must win over detection.
[ "$(select_frontend_mode 1)" = "standalone" ] \
    || fail "--no-plesk-integration must force standalone"

# ───────────────── standalone: unchanged behaviour ─────────────────

STANDALONE="$TEST_TMP/standalone.caddy"
render standalone > "$STANDALONE"

# Bare site addresses: Caddy is the edge and serves https on 443 itself.
for site in "remote.example.com {" "code.remote.example.com {" \
            "*.code.remote.example.com {" "*.dev.remote.example.com {"; do
    grep -qxF "$site" "$STANDALONE" || fail "standalone is missing site address '$site'"
done
grep -q "on_demand_tls {" "$STANDALONE" || fail "standalone lost on_demand_tls"
grep -q "ask http://127.0.0.1:7682/internal/tls-ask" "$STANDALONE" \
    || fail "standalone lost the tls-ask endpoint"
[ "$(grep -c "on_demand$" "$STANDALONE")" -eq 2 ] \
    || fail "standalone should have on-demand TLS on both wildcard blocks"

# Nothing from the Plesk topology may leak into a standalone install — this is
# the whole promise of gating it behind detection.
for absent in "auto_https" "default_bind" "trusted_proxies" ":8080" "http://remote"; do
    grep -q "$absent" "$STANDALONE" && fail "standalone must not contain '$absent'"
done

# ───────────────── plesk: loopback behind nginx ─────────────────

PLESK="$TEST_TMP/plesk.caddy"
render plesk > "$PLESK"

for site in "http://remote.example.com:8080 {" "http://code.remote.example.com:8080 {" \
            "http://*.code.remote.example.com:8080 {" "http://*.dev.remote.example.com:8080 {"; do
    grep -qxF "$site" "$PLESK" || fail "plesk is missing site address '$site'"
done
grep -q "auto_https off" "$PLESK" || fail "plesk mode must disable automatic https"
grep -q "default_bind 127.0.0.1" "$PLESK" \
    || fail "plesk mode must bind loopback, or the platform is served in plaintext publicly"
grep -q "trusted_proxies static 127.0.0.1/8 ::1" "$PLESK" \
    || fail "plesk mode must trust nginx, or X-Forwarded-Proto is overwritten with http"

# Caddy has no certificate to issue in this topology, and asking it to would
# fail ACME's HTTP-01 challenge on a port it does not own.
for absent in "on_demand" "tls-ask"; do
    grep -q "$absent" "$PLESK" && fail "plesk mode must not contain '$absent'"
done

# ───────────────── invariants across both ─────────────────

# The routing below the address line is what makes previews, the IDE and the
# inspector work, and it must be identical in both topologies.
for f in "$STANDALONE" "$PLESK"; do
    grep -q 'reverse_proxy 127.0.0.1:7682' "$f" || fail "$f lost the backend upstream"
    grep -q 'reverse_proxy {re.host.1}.lxd:{re.host.2}' "$f" || fail "$f lost dev-URL routing"
    grep -q 'reverse_proxy {re.host.1}.lxd:8842' "$f" || fail "$f lost per-project IDE routing"
    grep -q 'forward_auth 127.0.0.1:7682' "$f" || fail "$f lost forward_auth"
    # The host matchers anchor on the bare hostname with no port. In Plesk mode
    # that only holds because nginx passes Host through unmodified.
    grep -q 'header_regexp host Host \^(\[a-z0-9\]\[a-z0-9-\]\*)--(.d{4,5})\\.dev\\.remote\\.example\\.com\$' "$f" \
        || grep -qF 'header_regexp host Host ^([a-z0-9][a-z0-9-]*)--(\d{4,5})\.dev\.remote\.example\.com$' "$f" \
        || fail "$f lost the dev-URL host matcher"
    # An unrendered variable means the allowlist and the template disagree.
    if grep -qE '\$\{?[A-Z_][A-Z0-9_]*\}?' "$f"; then
        fail "$f has unrendered variables: $(grep -oE '\$\{?[A-Z_][A-Z0-9_]*\}?' "$f" | sort -u | tr '\n' ' ')"
    fi
done

# ───────────────── Caddy's own opinion ─────────────────

if command -v caddy >/dev/null 2>&1; then
    for f in "$STANDALONE" "$PLESK"; do
        caddy validate --config "$f" --adapter caddyfile >/dev/null 2>&1 \
            || fail "caddy validate rejected $f"
    done
else
    echo "SKIP: caddy not installed, config validated structurally only" >&2
fi

echo "PASS: caddyfile-render"
