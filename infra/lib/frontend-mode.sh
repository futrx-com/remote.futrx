# Which topology are we installing, and what does the Caddyfile look like in it?
#
#   standalone  Caddy owns 80/443 and manages its own certificates, including
#               on-demand issuance for the wildcard blocks. The original and
#               still the default.
#   plesk       Plesk's nginx already owns 80/443, so it terminates TLS and
#               proxies to Caddy on a loopback port.
#
# The public origin is https://<hostname> with no port in both cases, which is
# the reason this fits in a handful of variables: BASE_URL, the frontend's
# preview URL builder, the backend's agent-browser URL and the Caddy host
# matchers all keep working untouched. Only the listener and who holds the
# certificate move.
#
# Sourced by infra/install.sh and by infra/tests/caddyfile-render-test.sh, so
# the render the tests check is the render the installer produces.
#
# Requires lib/plesk.sh to have been sourced first.

# select_frontend_mode <no_plesk_integration>
# Prints "plesk" or "standalone".
select_frontend_mode() {
    local opt_out="${1:-0}"
    if [ "$opt_out" -eq 0 ] && plesk_present; then
        printf 'plesk\n'
    else
        printf 'standalone\n'
    fi
}

# set_caddy_render_vars <mode> <service_port> <caddy_http_port>
# Exports the five variables the Caddyfile template renders. In standalone
# mode every one of them produces the file this template rendered before the
# Plesk work existed — the directives are byte-identical, only the surrounding
# comments changed.
set_caddy_render_vars() {
    local mode="${1:-standalone}" service_port="${2:-7682}" caddy_port="${3:-8080}"

    if [ "$mode" = "plesk" ]; then
        CADDY_SITE_SCHEME="http://"
        CADDY_SITE_PORT=":${caddy_port}"
        # Plesk holds the certificate, so there is no on-demand issuance to gate.
        CADDY_TLS_BLOCK=""
        CADDY_GLOBAL_EXTRA="$(cat <<EOF
    # Plesk's nginx terminates TLS and proxies here over loopback, so Caddy
    # must not manage certificates or redirect to https itself.
    auto_https off
    # Loopback only. Without this Caddy would also answer on the public
    # interface at :${caddy_port}, serving the entire platform in plaintext
    # around Plesk's TLS.
    default_bind 127.0.0.1
    servers {
        # Preserve nginx's X-Forwarded-Proto: https. Left untrusted, Caddy
        # overwrites it with its own scheme (http), the backend's return_to
        # policy then rejects every non-https redirect target, and sign-in
        # silently stops returning users to where they started.
        trusted_proxies static 127.0.0.1/8 ::1
    }
EOF
)"
    else
        CADDY_SITE_SCHEME=""
        CADDY_SITE_PORT=""
        CADDY_TLS_BLOCK="$(printf '    tls {\n        on_demand\n    }')"
        CADDY_GLOBAL_EXTRA="$(cat <<EOF
    # On-demand TLS for the wildcard blocks. Caddy queries the backend's
    # /internal/tls-ask before issuing each cert so attackers can't burn
    # through ACME rate limits by hitting random subdomains.
    on_demand_tls {
        ask http://127.0.0.1:${service_port}/internal/tls-ask
    }
EOF
)"
    fi

    export CADDY_SITE_SCHEME CADDY_SITE_PORT CADDY_TLS_BLOCK CADDY_GLOBAL_EXTRA
}
