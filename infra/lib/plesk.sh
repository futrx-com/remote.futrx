# Plesk detection.
#
# A Plesk box is not a fresh server. Its nginx already owns 80 and 443, its
# BIND already owns 53, its firewall module reconciles iptables on its own
# schedule, and its SFTP users depend on SSH password auth. Every one of those
# collides with something the installer does by default.
#
# Rather than sprinkle "is Plesk here?" through the steps, this decides once
# and exports FUTRX_FRONTEND_MODE. Everything downstream branches on that, and
# a host without Plesk takes exactly the path it took before this file
# existed.
#
# Detection is by installation marker, not by process or port: the panel can
# be stopped, mid-upgrade, or listening somewhere unusual, and we still need
# to stay out of its way.

FUTRX_PLESK_ROOTS="${FUTRX_PLESK_ROOTS:-/usr/local/psa /opt/psa}"

# plesk_root
# Prints the Plesk installation root, or nothing when Plesk is absent.
plesk_root() {
    local root
    for root in $FUTRX_PLESK_ROOTS; do
        if [ -r "$root/version" ]; then
            printf '%s\n' "$root"
            return 0
        fi
    done
    return 1
}

# plesk_present
# Succeeds when this host runs Plesk.
plesk_present() {
    plesk_root >/dev/null 2>&1 || command -v plesk >/dev/null 2>&1
}

# plesk_version
# Prints the panel version, e.g. "18.0.62". The version file is
# "<version> <os> <build>"; only the first field is stable enough to report.
plesk_version() {
    local root
    root="$(plesk_root)" || return 1
    awk '{print $1; exit}' "$root/version" 2>/dev/null
}

# plesk_listen_ips
# Prints the IPv4 addresses Plesk's nginx binds for HTTPS, one per line.
#
# This matters more than it looks. nginx picks a server block by server_name
# only among the blocks sharing a listen socket, and the kernel routes a
# connection to the most specific bound socket. If Plesk binds 1.2.3.4:443 and
# we bind *:443, traffic to 1.2.3.4 never reaches our block and our hostnames
# fall through to whatever Plesk's default server does — which looks like a DNS
# or certificate problem, not a listen-address problem.
plesk_listen_ips() {
    [ -d /etc/nginx/plesk.conf.d ] || return 0
    grep -rhoE '^[[:space:]]*listen[[:space:]]+([0-9]{1,3}\.){3}[0-9]{1,3}:443' \
        /etc/nginx/plesk.conf.d 2>/dev/null \
        | grep -oE '([0-9]{1,3}\.){3}[0-9]{1,3}' \
        | sort -u
}

# cert_covers <cert-file> <name>...
# Succeeds when the certificate's SANs cover every name given, honouring
# wildcards. Used to refuse a certificate that covers the main hostname but
# not the wildcards — that combination produces a site which loads fine and
# then fails opaquely on every preview and IDE URL, which is a much worse
# failure than refusing to configure TLS at all.
cert_covers() {
    local cert="${1:-}"; shift
    [ -r "$cert" ] || return 1
    command -v openssl >/dev/null 2>&1 || return 1

    local sans name san matched
    sans="$(openssl x509 -in "$cert" -noout -ext subjectAltName 2>/dev/null \
        | tr ',' '\n' | sed -n 's/.*DNS://p' | tr -d ' ' | tr 'A-Z' 'a-z')"
    [ -n "$sans" ] || return 1

    for name in "$@"; do
        name="$(printf '%s' "$name" | tr 'A-Z' 'a-z')"
        matched=0
        for san in $sans; do
            if [ "$san" = "$name" ]; then
                matched=1
                break
            fi
            # A wildcard SAN covers exactly one label. "*.dev.example.com"
            # covers "a.dev.example.com" but not "a.b.dev.example.com", and a
            # literal "*.dev.example.com" request is satisfied by it too.
            case "$san" in
                \*.*)
                    if [ "$name" = "$san" ]; then matched=1; break; fi
                    case "$name" in
                        *.*)
                            if [ "${name#*.}" = "${san#\*.}" ] && [ "${name%%.*}" != "" ]; then
                                matched=1
                                break
                            fi
                            ;;
                    esac
                    ;;
            esac
        done
        [ "$matched" -eq 1 ] || return 1
    done
    return 0
}

# plesk_cert_for <name>...
# Prints the path of a certificate in Plesk's store that covers every name, or
# nothing. Plesk stores key, certificate and chain concatenated in one file,
# which nginx accepts for both ssl_certificate and ssl_certificate_key.
plesk_cert_for() {
    local root store cert
    root="$(plesk_root)" || return 1
    store="$root/var/certificates"
    [ -d "$store" ] || return 1
    for cert in "$store"/*; do
        [ -f "$cert" ] || continue
        if cert_covers "$cert" "$@"; then
            printf '%s\n' "$cert"
            return 0
        fi
    done
    return 1
}
