#!/usr/bin/env bash
set -euo pipefail

INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_TMP="$(mktemp -d)"
trap 'rm -rf "$TEST_TMP"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

export HOSTNAME="remote.example.com"
export CADDY_HTTP_PORT="8080"

ALLOWLIST='$HOSTNAME $CADDY_HTTP_PORT $PLESK_LISTEN $PLESK_TLS $PLESK_HTTP_SERVER'

render() {
    envsubst "$ALLOWLIST" < "$INFRA_DIR/templates/plesk-nginx.conf.tmpl"
}

# ───────────────── the TLS-configured shape ─────────────────

export PLESK_LISTEN="    listen 1.2.3.4:443 ssl;"
export PLESK_TLS="    ssl_certificate     /opt/psa/var/certificates/cert-abc;
    ssl_certificate_key /opt/psa/var/certificates/cert-abc;
"
export PLESK_HTTP_SERVER="
server {
    listen 1.2.3.4:80;
    server_name ${HOSTNAME};
    return 301 https://\$host\$request_uri;
}"

CONF="$TEST_TMP/tls.conf"
render > "$CONF"

# nginx picks a server block by server_name only among blocks sharing a listen
# socket, and the kernel routes to the most specific bound socket. Binding
# *:443 while Plesk binds 1.2.3.4:443 means our block is never consulted for
# that address, and the symptom looks like a DNS or certificate problem.
grep -qF "listen 1.2.3.4:443 ssl;" "$CONF" \
    || fail "the rendered config does not bind the address Plesk binds"
grep -qF "ssl_certificate     /opt/psa/var/certificates/cert-abc;" "$CONF" \
    || fail "certificate not rendered"

# All four host classes must be claimed, or they fall through to Plesk's
# default server.
for name in "${HOSTNAME}" "code.${HOSTNAME}" "*.code.${HOSTNAME}" "*.dev.${HOSTNAME}"; do
    grep -qF "$name" "$CONF" || fail "server_name is missing $name"
done

grep -qF "proxy_pass http://127.0.0.1:8080;" "$CONF" || fail "wrong upstream"

# Host must arrive unmodified and without a port: Caddy selects its site block
# from it, its matchers are anchored on the bare hostname, and the backend
# gates /__remote_inspector on the raw Host header.
grep -qF 'proxy_set_header Host $host;' "$CONF" || fail "Host is not passed through as \$host"
grep -q 'proxy_set_header Host \$http_host' "$CONF" && fail "\$http_host carries a port"
grep -q 'proxy_set_header Host \$proxy_host' "$CONF" && fail "\$proxy_host is the upstream, not the request"

# Without this Caddy overwrites the header with its own scheme (http), the
# backend rejects every return_to as a non-https redirect target, and the
# agent browser URL comes out as http://.
grep -qF "proxy_set_header X-Forwarded-Proto https;" "$CONF" \
    || fail "X-Forwarded-Proto is not forced to https"

# nginx is the real edge, so overwriting rather than appending takes the
# backend's rate-limit key out of the client's control.
grep -qF 'proxy_set_header X-Forwarded-For $remote_addr;' "$CONF" \
    || fail "X-Forwarded-For must be overwritten, not appended"
grep -q 'proxy_add_x_forwarded_for' "$CONF" \
    && fail "appending X-Forwarded-For leaves the leftmost element client-controlled"

# WebSockets. The map name is deliberately unique so it cannot collide with
# one Plesk or another conf.d file defines.
grep -qF "map \$http_upgrade \$futrx_connection_upgrade" "$CONF" || fail "missing the upgrade map"
grep -qF "proxy_http_version 1.1;" "$CONF" || fail "HTTP/1.1 is required for upgrades"
grep -qF 'proxy_set_header Upgrade $http_upgrade;' "$CONF" || fail "missing Upgrade header"
grep -qF 'proxy_set_header Connection $futrx_connection_upgrade;' "$CONF" \
    || fail "Connection header does not use our map"

# The chat and workspace sockets ping every 25s and would survive the 60s
# default; the terminal and tmux sockets are silent while idle and would not.
for directive in proxy_read_timeout proxy_send_timeout; do
    grep -qF "$directive 3600s;" "$CONF" \
        || fail "$directive must be raised, or idle terminals die after a minute"
done
grep -qF "proxy_buffering off;" "$CONF" || fail "streamed agent output must not be buffered"

# The redirect server only exists when there is a certificate to redirect to.
grep -qF 'return 301 https://$host$request_uri;' "$CONF" || fail "missing the http redirect"

# ───────────────── the HTTP-only fallback ─────────────────

# Rendered when no certificate covers the wildcards. Serving :443 with a
# certificate that covers only the main hostname would be worse: the UI would
# load and every preview would fail with an error nobody can act on.
export PLESK_LISTEN="    listen 1.2.3.4:80;"
export PLESK_TLS=""
export PLESK_HTTP_SERVER=""

HTTP_CONF="$TEST_TMP/http.conf"
render > "$HTTP_CONF"

grep -qF "listen 1.2.3.4:80;" "$HTTP_CONF" || fail "http-only config does not listen on 80"
grep -q "ssl_certificate" "$HTTP_CONF" && fail "http-only config must not reference a certificate"
grep -q "listen .*443" "$HTTP_CONF" && fail "http-only config must not listen on 443"
grep -q "return 301" "$HTTP_CONF" && fail "http-only config must not redirect to https"
# The proxy behaviour is otherwise identical, so routing can be verified now
# and the certificate added later.
grep -qF "proxy_pass http://127.0.0.1:8080;" "$HTTP_CONF" || fail "http-only config lost the upstream"

# ───────────────── no unrendered variables ─────────────────

for f in "$CONF" "$HTTP_CONF"; do
    if grep -qE '\$\{[A-Z_][A-Z0-9_]*\}' "$f"; then
        fail "$f has unrendered variables: $(grep -oE '\$\{[A-Z_][A-Z0-9_]*\}' "$f" | sort -u | tr '\n' ' ')"
    fi
done

if command -v nginx >/dev/null 2>&1; then
    echo "NOTE: nginx present but not validated — nginx -t needs the full server config" >&2
fi

echo "PASS: plesk-nginx-render"
