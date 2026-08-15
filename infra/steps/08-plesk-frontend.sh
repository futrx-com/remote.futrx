#!/usr/bin/env bash
# Put Plesk's nginx in front of Caddy.
#
# No-op unless FUTRX_FRONTEND_MODE is "plesk". On every other host this step
# does nothing at all and the install is exactly what it was before.
#
# Plesk owns 80 and 443 and will not give them up, so the topology becomes:
#
#   internet :443 → Plesk nginx (TLS) → 127.0.0.1:${CADDY_HTTP_PORT} Caddy → backend / containers
#
# The public origin stays https://${HOSTNAME} with no port, which is what lets
# every URL the platform generates keep working untouched.
#
# Expects from caller: log / warn / ok / err helpers, render_template,
# $INFRA_DIR, $HOSTNAME, $CADDY_HTTP_PORT, $PLESK_TLS_CERT, $PLESK_TLS_KEY.
set -euo pipefail

if [ "${FUTRX_FRONTEND_MODE:-standalone}" != "plesk" ]; then
    return 0 2>/dev/null || exit 0
fi

# shellcheck source=../lib/plesk.sh
. "$INFRA_DIR/lib/plesk.sh"

NGINX_CONF="/etc/nginx/conf.d/futrx.conf"

if ! command -v nginx >/dev/null 2>&1; then
    err "Plesk was detected but nginx is not installed."
    echo "  Enable nginx in Plesk (Tools & Settings → Services Management) and re-run." >&2
    exit 1
fi

# ───────────────── listen addresses ─────────────────
# nginx picks a server block by server_name only among the blocks sharing a
# listen socket, and the kernel hands a connection to the most specific bound
# socket. If Plesk binds 1.2.3.4:443 and we bind *:443, traffic to 1.2.3.4
# never reaches our block — our hostnames fall through to Plesk's default
# server and the symptom looks like a DNS or certificate problem instead of a
# listen-address one. So bind exactly what Plesk binds.
PLESK_IPS="$(plesk_listen_ips || true)"
if [ -z "$PLESK_IPS" ]; then
    warn "Could not read Plesk's HTTPS listen addresses; falling back to all interfaces."
    warn "If the site does not resolve to this config, check 'ss -tlnp | grep :443'."
    PLESK_LISTEN="    listen 443 ssl;
    listen [::]:443 ssl;"
    PLESK_LISTEN_HTTP="    listen 80;
    listen [::]:80;"
else
    PLESK_LISTEN=""
    PLESK_LISTEN_HTTP=""
    HAS_V6=0
    for ip in $PLESK_IPS; do
        case "$ip" in \[*\]) HAS_V6=1 ;; esac
        PLESK_LISTEN="${PLESK_LISTEN}${PLESK_LISTEN:+
}    listen ${ip}:443 ssl;"
        PLESK_LISTEN_HTTP="${PLESK_LISTEN_HTTP}${PLESK_LISTEN_HTTP:+
}    listen ${ip}:80;"
    done
    # Plesk's vhost templates do not always spell out an IPv6 listen even on a
    # dual-stack box. Without one here, every IPv6 request for our hostnames
    # falls through to Plesk's default vhost — the wrong site with the wrong
    # certificate, which reads as a DNS fault rather than a listen one.
    if [ "$HAS_V6" -eq 0 ]; then
        PLESK_LISTEN="${PLESK_LISTEN}
    listen [::]:443 ssl;"
        PLESK_LISTEN_HTTP="${PLESK_LISTEN_HTTP}
    listen [::]:80;"
    fi
    ok "binding the addresses Plesk uses: $(printf '%s' "$PLESK_IPS" | tr '\n' ' ')"
fi

# ───────────────── certificate ─────────────────
# Plesk terminates TLS, so it needs a certificate covering all four names.
# The wildcards are the hard part: they require a DNS-01 issuance, which is
# what Plesk's SSL It! extension does when it manages the zone.
NEEDED_NAMES=("$HOSTNAME" "code.$HOSTNAME" "*.code.$HOSTNAME" "*.dev.$HOSTNAME")

if [ -n "${PLESK_TLS_CERT:-}" ]; then
    if [ ! -r "$PLESK_TLS_CERT" ] || [ ! -r "$PLESK_TLS_KEY" ]; then
        err "--plesk-tls-cert / --plesk-tls-key are not readable."
        exit 1
    fi
    if ! cert_covers "$PLESK_TLS_CERT" "${NEEDED_NAMES[@]}"; then
        # Refusing is the kinder failure. A certificate covering only the main
        # hostname produces a site that loads, and then fails opaquely on every
        # preview and IDE URL with a certificate error users cannot diagnose.
        err "The certificate at $PLESK_TLS_CERT does not cover all four names:"
        printf '    %s\n' "${NEEDED_NAMES[@]}" >&2
        echo "  Wildcards require a DNS-01 issuance. Re-run once you have one." >&2
        exit 1
    fi
    ok "using the certificate passed on the command line"
elif PLESK_TLS_CERT="$(plesk_cert_for "${NEEDED_NAMES[@]}")"; then
    # Plesk stores key, certificate and chain concatenated in one file, which
    # nginx accepts for both directives.
    PLESK_TLS_KEY="$PLESK_TLS_CERT"
    ok "found a covering certificate in Plesk's store: $PLESK_TLS_CERT"
else
    PLESK_TLS_CERT=""
    PLESK_TLS_KEY=""
fi

if [ -n "$PLESK_TLS_CERT" ]; then
    PLESK_TLS="    ssl_certificate     ${PLESK_TLS_CERT};
    ssl_certificate_key ${PLESK_TLS_KEY};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:futrx_ssl:10m;
    ssl_session_timeout 1d;
"
    PLESK_HTTP_SERVER="
server {
${PLESK_LISTEN_HTTP}
    server_name ${HOSTNAME} code.${HOSTNAME} *.code.${HOSTNAME} *.dev.${HOSTNAME};
    return 301 https://\$host\$request_uri;
}"
else
    # HTTP-only, deliberately. Routing can be verified end to end now, and the
    # operator gets a certificate afterwards and re-runs. Serving :443 with a
    # certificate that does not cover the wildcards would be worse: the main
    # UI would work and every preview would fail with an error nobody can act
    # on.
    warn "No certificate covering the wildcards was found — configuring HTTP only."
    PLESK_LISTEN="$PLESK_LISTEN_HTTP"
    PLESK_TLS=""
    PLESK_HTTP_SERVER=""
    NEEDS_CERT=1
fi

export PLESK_LISTEN PLESK_TLS PLESK_HTTP_SERVER PLESK_TLS_CERT PLESK_TLS_KEY

# ───────────────── render, validate, install ─────────────────
# Same shape as steps/03-caddy.sh: validate a candidate before it can take the
# site down, and reload only when something actually changed.
log "Rendering $NGINX_CONF"
# No EXIT trap: this file is sourced by the installer, so a trap here would
# outlive the step and replace whatever the caller had.
TMP_CONF="$(mktemp)"
render_template "${INFRA_DIR}/templates/plesk-nginx.conf.tmpl" "$TMP_CONF"

# nginx can only check a config in place, so validate against a staged copy of
# the real file and restore whatever was there if it does not pass.
BACKUP=""
if [ -e "$NGINX_CONF" ]; then
    BACKUP="$(mktemp)"
    cp -p "$NGINX_CONF" "$BACKUP"
fi
CONF_CHANGED=0
if [ ! -e "$NGINX_CONF" ] || ! cmp -s "$TMP_CONF" "$NGINX_CONF"; then
    CONF_CHANGED=1
fi

install -m 0644 "$TMP_CONF" "$NGINX_CONF"
rm -f "$TMP_CONF"
if ! nginx -t >/dev/null 2>&1; then
    err "nginx rejected the generated config; reverting."
    nginx -t 2>&1 | sed 's/^/    /' >&2
    if [ -n "$BACKUP" ]; then
        install -m 0644 "$BACKUP" "$NGINX_CONF"
    else
        rm -f "$NGINX_CONF"
    fi
    rm -f "$BACKUP"
    exit 1
fi
rm -f "$BACKUP"

if [ "$CONF_CHANGED" -eq 1 ]; then
    systemctl reload nginx || systemctl restart nginx
    ok "nginx reloaded with $NGINX_CONF"
else
    ok "$NGINX_CONF already current"
fi

if [ "${NEEDS_CERT:-0}" -eq 1 ]; then
    cat >&2 <<EOF

  ─────────────────────────────────────────────────────────────
  TLS is not configured yet. The platform is reachable over HTTP
  so you can verify routing, but sign-in will not work: session
  cookies are Secure-only.

  Plesk needs one certificate covering all four names:
    ${HOSTNAME}
    code.${HOSTNAME}
    *.code.${HOSTNAME}
    *.dev.${HOSTNAME}

  The wildcards require a DNS-01 issuance, which means Plesk must
  manage this zone. In Plesk: add ${HOSTNAME} as a domain, then
  SSL It! → Let's Encrypt → tick both wildcard options.

  Then re-run this installer. It will find the certificate in
  Plesk's store and switch to HTTPS automatically. To use a
  certificate from elsewhere instead:
    --plesk-tls-cert=/path/to/fullchain.pem --plesk-tls-key=/path/to/key.pem
  ─────────────────────────────────────────────────────────────

EOF
fi
