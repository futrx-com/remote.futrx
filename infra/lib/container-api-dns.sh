# Pin the public API hostname to the host's LXD gateway without changing TLS.
# Only the marked block is owned by Remote; other raw.dnsmasq settings survive.
# no-hosts stops exposing the host's /etc/hosts aliases to project containers.
# LXD's DHCP/container records and explicit custom host-records still work.

# ensure_container_api_dns <bridge> <hostname> <bridge IPv4>
# Returns non-zero on invalid input, failed configuration, or wrong DNS answers.
ensure_container_api_dns() {
    local bridge="$1" hostname="$2" address="$3"
    local previous merged answers ipv4 ipv6 attempt
    local begin='# BEGIN remote.futrx container API DNS'
    local end='# END remote.futrx container API DNS'

    # Reject config delimiters/newlines and disabled/automatic IPv4 settings.
    if ! [[ "$hostname" =~ ^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$ ]] ||
       ! awk -F. 'NF != 4 {exit 1} {for (i=1;i<=4;i++) if ($i !~ /^[0-9]+$/ || $i > 255) exit 1}
           $1 == 0 || $1 == 127 || $1 >= 224 {exit 1}' <<<"$address"; then
        echo "Invalid container API DNS hostname or bridge IPv4 address." >&2
        return 1
    fi
    if ! previous="$(lxc network get "$bridge" raw.dnsmasq)"; then
        echo "Could not read $bridge raw.dnsmasq." >&2
        return 1
    fi
    # Refuse malformed ownership markers rather than discarding custom config.
    if ! merged="$(awk -v begin="$begin" -v end="$end" '
        $0 == begin {if (managed) exit 1; managed=1; next}
        $0 == end {if (!managed) exit 1; managed=0; next}
        !managed {print}
        END {if (managed) exit 1}
    ' <<<"$previous")"; then
        echo "Malformed Remote DNS block in $bridge raw.dnsmasq." >&2
        return 1
    fi
    merged="${merged:+$merged$'\n'}$begin"$'\nno-hosts\n'"host-record=$hostname,$address"$'\n'"$end"
    if [ "$merged" != "$previous" ]; then
        # LXD applies the setting and reloads its network DNS service.
        if ! lxc network set "$bridge" raw.dnsmasq "$merged"; then
            echo "Could not configure $bridge container API DNS." >&2
            return 1
        fi
    fi

    # Query the exact resolver advertised to containers, not the host's NSS.
    # A host-record with only IPv4 must also suppress upstream AAAA answers.
    for attempt in 1 2 3 4 5; do
        if ipv4="$(dig "@$address" "$hostname." A +time=1 +tries=1 +noall +comments +answer)" &&
           ipv6="$(dig "@$address" "$hostname." AAAA +time=1 +tries=1 +noall +comments +answer)" &&
           [[ "$ipv4" == *'status: NOERROR,'* ]] &&
           [[ "$ipv6" == *'status: NOERROR,'* ]]; then
            answers="$(awk '!/^;/ && NF {print $4 " " $5}' <<<"$ipv4")"
            if [ "$answers" = "A $address" ] &&
               [ -z "$(awk '!/^;/ && NF {print}' <<<"$ipv6")" ]; then
                return 0
            fi
        fi
        [ "$attempt" -eq 5 ] || sleep 1
    done
    echo "Bridge DNS verification failed: $hostname must resolve only to $address (no AAAA)." >&2
    echo "Inspect: dig @$address $hostname A; dig @$address $hostname AAAA" >&2
    if [ "$merged" != "$previous" ]; then
        lxc network set "$bridge" raw.dnsmasq "$previous" ||
            echo "Could not restore previous $bridge raw.dnsmasq; inspect the network configuration." >&2
    fi
    return 1
}
