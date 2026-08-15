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

# Candidate BIND user-includes, most specific first. The `options:` ones are
# spliced inside BIND's options statement, so directives go in bare; the
# `toplevel:` ones need their own wrapping options block.
FUTRX_NAMED_INCLUDE_CANDIDATES="${FUTRX_NAMED_INCLUDE_CANDIDATES:-\
options:/var/named/run-root/etc/named.user.options.conf \
options:/etc/named.user.options.conf \
toplevel:/var/named/run-root/etc/named.user.conf \
toplevel:/etc/named.user.conf}"

# bridge_dns_listener <bridge_ip>
# Prints the program holding <bridge_ip>:53, or nothing when the address is
# free. Checks UDP, which is what DHCP and DNS resolution actually need.
#
# An explicit bind to the bridge address always wins over a wildcard one, and
# not merely because it is more specific: the two can coexist (dnsmasq on
# 10.x.x.1:53 alongside an unrelated resolver on 0.0.0.0:53 is a normal state
# on a box running pi-hole or unbound), and taking whichever `ss` happened to
# print first would report a healthy bridge as hijacked — then rewrite BIND's
# config, or abort the install outright.
bridge_dns_listener() {
    local bridge_ip="${1:-}" exact wildcard
    [ -n "$bridge_ip" ] || return 1
    command -v ss >/dev/null 2>&1 || return 1

    local parsed
    parsed="$(ss -lnup 2>/dev/null | awk -v ip="$bridge_ip" '
        NR == 1 { next }
        {
            # State Recv-Q Send-Q Local:Port Peer:Port users:((...))
            split($4, a, ":")
            port = a[length(a)]
            if (port != "53") next
            addr = substr($4, 1, length($4) - length(port) - 1)
            if (addr == ip) { kind = "exact" }
            else if (addr == "0.0.0.0" || addr == "*" || addr == "[::]") { kind = "wildcard" }
            else next
            if (match($0, /users:\(\("[^"]+/)) {
                s = substr($0, RSTART, RLENGTH)
                sub(/^users:\(\("/, "", s)
                print kind, s
            }
        }
    ')"

    exact="$(printf '%s\n' "$parsed" | awk '$1 == "exact" { print $2; exit }')"
    if [ -n "$exact" ]; then
        printf '%s\n' "$exact"
        return 0
    fi
    wildcard="$(printf '%s\n' "$parsed" | awk '$1 == "wildcard" { print $2; exit }')"
    [ -z "$wildcard" ] || printf '%s\n' "$wildcard"
    return 0
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
# Prints "<scope> <path>" for Plesk's BIND user-include, or nothing. Plesk
# regenerates /etc/named.conf on every DNS change but preserves these
# includes, so they are the only place a change of ours can survive.
#
# Scope decides the syntax. BIND permits exactly one `options` statement in a
# configuration, so an options block in a *top-level* include is a syntax
# error on any layout where Plesk already has one — which is most of them.
# An options-scoped include takes the directive bare and always works.
#
# A candidate must already exist to be used. Creating one ourselves would
# write a file nothing includes: named-checkconf would pass, BIND would
# restart happily, and nothing would have changed — the worst outcome
# available, since it reports success and fixes nothing.
named_user_include() {
    local candidate scope path
    for candidate in $FUTRX_NAMED_INCLUDE_CANDIDATES; do
        scope="${candidate%%:*}"
        path="${candidate#*:}"
        if [ -f "$path" ]; then
            printf '%s %s\n' "$scope" "$path"
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
    local bridge_ip="${1:-}" resolved scope include backup marker
    [ -n "$bridge_ip" ] || return 1
    command -v named-checkconf >/dev/null 2>&1 || return 1
    resolved="$(named_user_include)" || return 1
    scope="${resolved%% *}"
    include="${resolved#* }"
    marker="# remote.futrx: leave ${bridge_ip} to LXD's dnsmasq"

    if grep -qF "$marker" "$include" 2>/dev/null; then
        return 0
    fi

    backup="${include}.futrx-bak"
    cp -p "$include" "$backup" || return 1

    {
        cat "$include"
        printf '%s\n' \
            "" \
            "$marker" \
            "# BIND's default listen-on { any; } binds every interface address it finds," \
            "# including the bridge, which stops LXD's dnsmasq from serving DHCP there" \
            "# and leaves containers with IPv6 only."
        # An options-scoped include is spliced inside BIND's own options
        # statement, so the directive goes in bare. Wrapping it would be a
        # nested-options syntax error; not wrapping it in a top-level include
        # would be a stray directive. Either way named-checkconf catches it and
        # the rollback below runs, but getting it right is what makes the
        # automatic path actually work.
        if [ "$scope" = "options" ]; then
            printf '%s\n' "listen-on port 53 { !${bridge_ip}; any; };"
        else
            printf '%s\n' \
                "options {" \
                "    listen-on port 53 { !${bridge_ip}; any; };" \
                "};"
        fi
    } > "${include}.futrx-new" || return 1
    mv "${include}.futrx-new" "$include" || return 1

    if ! named-checkconf >/dev/null 2>&1; then
        mv "$backup" "$include"
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
# does nothing — LXD short-circuits an update whose config did not change — so
# the nudge has to be a real change.
#
# raw.dnsmasq is the right key to toggle: it lands in the dnsmasq config file,
# so LXD definitely restarts the process, and a stray comment left behind by an
# interrupted run is inert. Toggling ipv4.nat would work too, but a run
# interrupted between the two writes would leave masquerading *off* — which is
# the "address but no egress" outage this whole file exists to diagnose, and a
# worse state than the one we started in. It would also drop NAT for every
# running container for the duration.
restart_bridge_dns() {
    local bridge="${1:-}" previous
    [ -n "$bridge" ] || return 1
    command -v lxc >/dev/null 2>&1 || return 1

    previous="$(lxc network get "$bridge" raw.dnsmasq 2>/dev/null || true)"
    lxc network set "$bridge" raw.dnsmasq "# remote.futrx: restarting dnsmasq" \
        >/dev/null 2>&1 || return 1
    if [ -n "$previous" ]; then
        lxc network set "$bridge" raw.dnsmasq "$previous" >/dev/null 2>&1 || return 1
    else
        lxc network unset "$bridge" raw.dnsmasq >/dev/null 2>&1 || return 1
    fi
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
