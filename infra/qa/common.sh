#!/usr/bin/env bash
# Shared connection, ref-validation, and public-health helpers for QA deploys.

set -euo pipefail

QA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
QA_REPO_ROOT="$(cd "$QA_DIR/../.." >/dev/null 2>&1 && pwd)"
QA_ENV_FILE="${QA_ENV_FILE:-$QA_REPO_ROOT/.qa.env}"

# Git Bash rewrites arguments that look like absolute Unix paths into Windows
# paths before handing them to a native executable, which would corrupt the
# remote /tmp paths these scripts pass to ssh and scp. Both variables are
# ignored outside MSYS, so exporting them unconditionally costs nothing on
# Linux or macOS.
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

qa_fail() {
    printf 'qa: %s\n' "$*" >&2
    exit 1
}

# Name of the IPv4 resolver available on this operator's machine, or empty.
# These scripts are driven from Linux, macOS, and Git Bash on Windows, and no
# single lookup tool ships on all three: getent is glibc-only, dscacheutil is
# macOS, and a Git Bash install reaches Windows' own nslookup. curl is already
# a hard requirement above, so DNS-over-HTTPS closes the gap on a machine that
# has none of them.
qa_ipv4_resolver() {
    local candidate

    for candidate in getent dscacheutil dig host nslookup curl; do
        if command -v "$candidate" >/dev/null 2>&1; then
            printf '%s' "$candidate"
            return 0
        fi
    done
    return 1
}

# qa_resolve_ipv4 <hostname> <resolver>
# Prints the first IPv4 address for the name, or nothing when it does not
# resolve. Callers must tolerate both an empty result and a non-zero exit; a
# name with no A record is a reportable condition here, not a script error.
qa_resolve_ipv4() {
    local name="$1" resolver="$2" doh body

    case "$resolver" in
        getent)
            getent ahostsv4 "$name" 2>/dev/null | awk 'NR == 1 { print $1 }'
            ;;
        dscacheutil)
            dscacheutil -q host -a name "$name" 2>/dev/null |
                awk '$1 == "ip_address:" { print $2; exit }'
            ;;
        dig)
            dig +short A "$name" 2>/dev/null |
                awk '/^([0-9]{1,3}[.]){3}[0-9]{1,3}$/ { print; exit }'
            ;;
        host)
            host -t A "$name" 2>/dev/null | awk '/has address/ { print $NF; exit }'
            ;;
        nslookup)
            # Windows and BIND nslookup both open with the DNS server's own
            # address, so only read addresses after the answer's Name: line.
            # Windows prints extra addresses on bare continuation lines under
            # "Addresses:", which is why every field is scanned.
            nslookup -type=A "$name" 2>/dev/null | awk '
                /^Name:/ { answer = 1; next }
                answer {
                    for (i = 1; i <= NF; i++) {
                        if ($i ~ /^([0-9]{1,3}[.]){3}[0-9]{1,3}$/) {
                            print $i
                            exit
                        }
                    }
                }
            '
            ;;
        curl)
            for doh in https://cloudflare-dns.com/dns-query https://dns.google/resolve; do
                body="$(curl -fsSL --max-time 8 -H 'accept: application/dns-json' \
                    "${doh}?name=${name}&type=A" 2>/dev/null)" || continue
                # A reply carrying Status is authoritative even when it lists
                # no address, so don't retry the next resolver on NXDOMAIN.
                printf '%s' "$body" | grep -q '"Status"' || continue
                printf '%s' "$body" |
                    grep -oE '"data"[[:space:]]*:[[:space:]]*"([0-9]{1,3}[.]){3}[0-9]{1,3}"' |
                    grep -oE '([0-9]{1,3}[.]){3}[0-9]{1,3}' |
                    awk 'NR == 1 { print }'
                return 0
            done
            ;;
    esac
}

qa_prepare_connection() {
    local command_name resolved_qa_ip resolver

    if [ ! -r "$QA_ENV_FILE" ]; then
        qa_fail "missing $QA_ENV_FILE; copy .qa.env.example to .qa.env and configure it"
    fi

    # shellcheck source=/dev/null
    . "$QA_ENV_FILE"

    : "${QA_SSH_HOST:?set QA_SSH_HOST in $QA_ENV_FILE}"
    : "${QA_SSH_USER:=root}"
    : "${QA_SSH_KEY:?set QA_SSH_KEY in $QA_ENV_FILE}"
    : "${QA_KNOWN_HOSTS_FILE:?set QA_KNOWN_HOSTS_FILE in $QA_ENV_FILE}"
    : "${QA_PUBLIC_HOST:?set QA_PUBLIC_HOST in $QA_ENV_FILE}"

    [ -r "$QA_SSH_KEY" ] || qa_fail "SSH private key is not readable: $QA_SSH_KEY"
    mkdir -p "$(dirname "$QA_KNOWN_HOSTS_FILE")"
    touch "$QA_KNOWN_HOSTS_FILE"
    chmod 600 "$QA_KNOWN_HOSTS_FILE"

    for command_name in ssh curl; do
        command -v "$command_name" >/dev/null 2>&1 || qa_fail "required command is missing: $command_name"
    done

    resolver="$(qa_ipv4_resolver || true)"
    [ -n "$resolver" ] || \
        qa_fail "required command is missing: one of getent, dscacheutil, dig, host, nslookup, curl"

    # An unresolvable name must reach the check below, not abort the script
    # through pipefail on the resolver's own non-zero exit.
    resolved_qa_ip="$(qa_resolve_ipv4 "$QA_PUBLIC_HOST" "$resolver" || true)"
    if [ -z "$resolved_qa_ip" ]; then
        qa_fail "$QA_PUBLIC_HOST does not resolve to an IPv4 address"
    fi
    if [ "$QA_SSH_HOST" != "$QA_PUBLIC_HOST" ] && [ "$resolved_qa_ip" != "$QA_SSH_HOST" ]; then
        qa_fail "$QA_PUBLIC_HOST resolves to $resolved_qa_ip, not QA_SSH_HOST $QA_SSH_HOST"
    fi

    QA_SSH_ARGS=(
        -i "$QA_SSH_KEY"
        -o BatchMode=yes
        -o ConnectTimeout=10
        -o StrictHostKeyChecking=yes
        -o "UserKnownHostsFile=$QA_KNOWN_HOSTS_FILE"
    )
}

qa_prepare() {
    local requested_ref="$1"

    case "$requested_ref" in
        '') qa_fail "a branch, tag, or commit is required" ;;
        *[!A-Za-z0-9._/@+-]*) qa_fail "ref contains unsupported characters: $requested_ref" ;;
    esac

    qa_prepare_connection

    command -v git >/dev/null 2>&1 || qa_fail "required command is missing: git"

    cd "$QA_REPO_ROOT"

    if ! git diff --quiet || ! git diff --cached --quiet; then
        qa_fail "tracked working-tree changes exist; commit or stash them before deploying"
    fi

    printf '==> Fetching %s from origin\n' "$requested_ref"
    git fetch --quiet origin "$requested_ref"
    QA_CANDIDATE_SHA="$(git rev-parse --verify 'FETCH_HEAD^{commit}')"
    QA_LOCAL_SHA="$(git rev-parse --verify 'HEAD^{commit}')"
    QA_REQUESTED_REF="$requested_ref"

    if [ "$QA_LOCAL_SHA" != "$QA_CANDIDATE_SHA" ]; then
        qa_fail "local HEAD $QA_LOCAL_SHA is not the pushed candidate $QA_CANDIDATE_SHA; check out the requested ref first"
    fi

    printf '==> Candidate: %s (%s)\n' "$QA_REQUESTED_REF" "$QA_CANDIDATE_SHA"
}

qa_verify_public_url() {
    printf '==> Verifying https://%s/\n' "$QA_PUBLIC_HOST"
    curl -fsS --max-time 20 "https://$QA_PUBLIC_HOST/" >/dev/null
}
