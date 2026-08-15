# LXD's dnsmasq vs whatever else on the box wants port 53.
#
# LXD runs a dnsmasq per bridge, bound to the bridge address, serving both DHCP
# and DNS to the containers on it. If something else already owns that address
# on :53, dnsmasq fails to bind and LXD brings the bridge up anyway — so
# containers boot, get no DHCP lease, and come up with IPv6 link-local only.
#
# That is indistinguishable from a firewall problem at the point where it
# hurts: the base-image build dies with "cannot reach any IPv4 address", and
# every forwarding fix in container-forwarding.sh correctly reports success
# because forwarding was never the problem. It is also why
# steps/07-lxc-ipv4-heal.sh cannot help — `networkctl reconfigure eth0` just
# re-runs a DHCP request that has no server to answer it.
#
# The usual squatter is BIND. Plesk installs and runs it for its DNS service,
# and BIND's default `listen-on { any; }` is implemented by binding each
# interface address individually and re-scanning periodically — so it picks up
# lxdbr0's address whenever the bridge appears, including after a reboot when
# it wins the race against LXD.
#
# Expects from caller: log / warn / ok / err helpers.

FUTRX_NAMED_INCLUDE_CANDIDATES="${FUTRX_NAMED_INCLUDE_CANDIDATES:-/var/named/run-root/etc/named.user.conf /etc/named.user.conf}"

# bridge_dns_listener <bridge_ip>
# Prints the program holding <bridge_ip>:53, or nothing when the address is
# free. Checks UDP, which is what DHCP and DNS resolution actually need.
bridge_dns_listener() {
    local bridge_ip="${1:-}"
    [ -n "$bridge_ip" ] || return 1
    command -v ss >/dev/null 2>&1 || return 1
    # A wildcard listener owns the bridge address too, so match both the
    # explicit address and 0.0.0.0/*.
    ss -lnup 2>/dev/null | awk -v ip="$bridge_ip" '
        NR == 1 { next }
        {
            # State Recv-Q Send-Q Local:Port Peer:Port users:((...))
            split($4, a, ":")
            port = a[length(a)]
            if (port != "53") next
            addr = substr($4, 1, length($4) - length(port) - 1)
            if (addr != ip && addr != "0.0.0.0" && addr != "*" && addr != "[::]") next
            if (match($0, /users:\(\("[^"]+/)) {
                s = substr($0, RSTART, RLENGTH)
                sub(/^users:\(\("/, "", s)
                print s
                exit
            }
        }
    '
}

# bridge_dns_healthy <bridge_ip>
# Succeeds when LXD's dnsmasq owns <bridge_ip>:53. Anything else holding it —
# or nothing at all — means containers will not get a lease.
bridge_dns_healthy() {
    local bridge_ip="${1:-}" holder
    [ -n "$bridge_ip" ] || return 1
    holder="$(bridge_dns_listener "$bridge_ip" || true)"
    [ "$holder" = "dnsmasq" ]
}

# named_user_include
# Prints the path of Plesk's supported BIND user-include. Plesk regenerates
# /etc/named.conf on every DNS change but preserves this include, so it is the
# only place a change of ours can survive.
#
# Whether an `options` statement is legal there depends on where Plesk splices
# the include: BIND permits exactly one options statement in the whole config,
# so on a layout that already has one at top level a second is a syntax error.
# That is why exclude_bridge_from_named validates with named-checkconf and
# rolls back — and why ensure_bridge_dns has a real remediation message for
# when it does not take, rather than treating the automatic path as the only
# one.
named_user_include() {
    local candidate
    for candidate in $FUTRX_NAMED_INCLUDE_CANDIDATES; do
        if [ -e "$candidate" ]; then
            printf '%s\n' "$candidate"
            return 0
        fi
    done
    for candidate in $FUTRX_NAMED_INCLUDE_CANDIDATES; do
        if [ -d "$(dirname "$candidate")" ]; then
            printf '%s\n' "$candidate"
            return 0
        fi
    done
    return 1
}

# exclude_bridge_from_named <bridge_ip>
# Tells BIND to stop binding the LXD bridge address, so dnsmasq can have it.
# Validates with named-checkconf and rolls the include back on any failure —
# a box whose BIND will not start has lost its DNS service, which is a far
# worse outcome than the container problem we are fixing.
exclude_bridge_from_named() {
    local bridge_ip="${1:-}" include backup marker
    [ -n "$bridge_ip" ] || return 1
    command -v named-checkconf >/dev/null 2>&1 || return 1
    include="$(named_user_include)" || return 1
    marker="# remote.futrx: leave ${bridge_ip} to LXD's dnsmasq"

    if [ -r "$include" ] && grep -qF "$marker" "$include"; then
        return 0
    fi

    backup=""
    if [ -e "$include" ]; then
        backup="${include}.futrx-bak"
        cp -p "$include" "$backup" || return 1
    fi

    {
        [ -e "$include" ] && cat "$include"
        printf '%s\n' \
            "" \
            "$marker" \
            "# BIND's default listen-on { any; } binds every interface address it finds," \
            "# including the bridge, which stops LXD's dnsmasq from serving DHCP there" \
            "# and leaves containers with IPv6 only." \
            "options {" \
            "    listen-on port 53 { !${bridge_ip}; any; };" \
            "};"
    } > "${include}.futrx-new" || return 1
    mv "${include}.futrx-new" "$include" || return 1

    if ! named-checkconf >/dev/null 2>&1; then
        if [ -n "$backup" ]; then
            mv "$backup" "$include"
        else
            rm -f "$include"
        fi
        return 1
    fi
    rm -f "$backup"
    return 0
}

# restart_named
# Restarts whichever unit name BIND is under on this box.
restart_named() {
    local unit
    for unit in named bind9; do
        if systemctl is-active --quiet "$unit" 2>/dev/null; then
            systemctl restart "$unit" >/dev/null 2>&1 && return 0
            return 1
        fi
    done
    return 1
}

# restart_bridge_dns <bridge>
# Makes LXD re-render the bridge, which re-spawns its dnsmasq. There is no
# "restart this network" verb, and setting a key to the value it already holds
# does nothing — LXD short-circuits an update whose config did not change. So
# this genuinely toggles ipv4.nat and lands on true, which is the value
# ensure_bridge_nat wants anyway.
restart_bridge_dns() {
    local bridge="${1:-}"
    [ -n "$bridge" ] || return 1
    command -v lxc >/dev/null 2>&1 || return 1
    lxc network set "$bridge" ipv4.nat false >/dev/null 2>&1 || return 1
    lxc network set "$bridge" ipv4.nat true >/dev/null 2>&1 || return 1
    return 0
}

# wait_bridge_dns_healthy <bridge_ip> [tries]
# LXD re-renders the bridge asynchronously, and dnsmasq can take a couple of
# seconds to bind after the network update returns. A fixed sleep would either
# be too short on a loaded box or waste time on an idle one.
wait_bridge_dns_healthy() {
    local bridge_ip="${1:-}" tries="${2:-10}" i=0
    while [ "$i" -lt "$tries" ]; do
        bridge_dns_healthy "$bridge_ip" && return 0
        i=$((i + 1))
        sleep 1
    done
    return 1
}

# ensure_bridge_dns <bridge> <bridge_ip>
# The whole repair, in the order that makes each step's failure legible.
# Returns non-zero when the bridge address is still not served by dnsmasq, so
# the caller can stop rather than proceed to a build that will fail minutes
# later for reasons that look nothing like this.
ensure_bridge_dns() {
    local bridge="${1:-}" bridge_ip="${2:-}" holder
    [ -n "$bridge" ] && [ -n "$bridge_ip" ] || return 1

    if bridge_dns_healthy "$bridge_ip"; then
        return 0
    fi

    holder="$(bridge_dns_listener "$bridge_ip" || true)"
    if [ -z "$holder" ]; then
        warn "Nothing is serving DNS/DHCP on ${bridge_ip}:53 — restarting the bridge."
        restart_bridge_dns "$bridge" || true
        wait_bridge_dns_healthy "$bridge_ip" && return 0
        return 1
    fi

    case "$holder" in
        named|bind|bind9)
            log "BIND holds ${bridge_ip}:53 — excluding the bridge so LXD's dnsmasq can bind"
            if exclude_bridge_from_named "$bridge_ip"; then
                restart_named || warn "BIND did not restart cleanly; check 'systemctl status named'."
                restart_bridge_dns "$bridge" || true
                wait_bridge_dns_healthy "$bridge_ip" && return 0
            fi
            err "BIND still holds ${bridge_ip}:53."
            bind_remediation "$bridge_ip"
            return 1
            ;;
        dnsmasq)
            return 0
            ;;
        *)
            err "${holder} holds ${bridge_ip}:53, which stops LXD from serving DHCP to containers."
            echo "  LXD needs that address for container DHCP and for .lxd name resolution," >&2
            echo "  which the IDE and preview routing both depend on, so it cannot be moved." >&2
            echo "  Stop or rebind ${holder} so it leaves ${bridge_ip} alone, then re-run." >&2
            return 1
            ;;
    esac
}

# bind_remediation <bridge_ip>
# What to do by hand when the automatic exclusion did not take. Both options
# are real; which one applies depends on whether this server actually serves
# DNS from Plesk.
bind_remediation() {
    local bridge_ip="${1:-}"
    cat >&2 <<EOF

  BIND binds every interface address it finds, including LXD's bridge, so
  LXD's dnsmasq cannot start and containers never get an IPv4 lease. They come
  up with IPv6 only, which looks exactly like a firewall problem and is not.

  LXD needs ${bridge_ip}:53 — it serves both container DHCP and .lxd name
  resolution, which the IDE and preview routing depend on — so BIND has to
  give it up. Two ways:

  1. If this server does not serve DNS from Plesk (external DNS, or a
     registrar's nameservers), turn the service off:
       Plesk → Tools & Settings → Services Management → stop and disable
       the DNS server. Or: systemctl disable --now named

  2. If Plesk does serve DNS, restrict what BIND listens on. In the options
     block of /etc/named.conf:
       listen-on port 53 { !${bridge_ip}; any; };
     then: named-checkconf && systemctl restart named
     Note that Plesk regenerates named.conf on DNS changes, so re-apply this
     if it reverts.

  Then re-run this installer.

EOF
}
