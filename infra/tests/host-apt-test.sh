#!/usr/bin/env bash
set -euo pipefail

INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$INFRA_DIR/lib/host-apt.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT
mkdir -p "$TEST_DIR/bin"
export PATH="$TEST_DIR/bin:$PATH"
export APT_TEST_DIR="$TEST_DIR"
export FUTRX_HOST_APT_GET_BIN="$TEST_DIR/bin/real-apt-get"

fail() { echo "FAIL: $*" >&2; exit 1; }

cat > "$TEST_DIR/bin/apt-config" <<'SCRIPT'
#!/usr/bin/env bash
printf 'source_list=%q\nsource_parts=%q\n' \
    "$APT_TEST_DIR/sources/sources.list" "$APT_TEST_DIR/sources/sources.list.d"
SCRIPT
cat > "$TEST_DIR/bin/caddy" <<'SCRIPT'
#!/usr/bin/env bash
[ "$CADDY_USABLE" = 1 ] || exit 1
echo 'v2.11.4'
SCRIPT
cat > "$FUTRX_HOST_APT_GET_BIN" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> "$APT_TEST_DIR/calls"
if [[ " $* " != *' update '* ]]; then exit 0; fi
[[ " $* " == *' APT::Update::Error-Mode=any '* ]] || exit 91
source_list="$APT_TEST_DIR/sources/sources.list"
source_parts="$APT_TEST_DIR/sources/sources.list.d"
for argument in "$@"; do
    case "$argument" in
        Dir::Etc::sourcelist=*) source_list="${argument#*=}" ;;
        Dir::Etc::sourceparts=*) source_parts="${argument#*=}" ;;
    esac
done
if [ "$source_list" != "$APT_TEST_DIR/sources/sources.list" ]; then
    view="${source_list%/*}"
    echo "$view" >> "$APT_TEST_DIR/views"
    rm -rf "$APT_TEST_DIR/captured-view"
    cp -a "$view" "$APT_TEST_DIR/captured-view"
    # Only Caddy's URI may disappear; custom sources and signature settings survive.
    grep -Fq 'https://archive.ubuntu.com/ubuntu noble main' "$source_list" || exit 92
    grep -Fq 'deb [signed-by=/custom.gpg] https://custom.example/apt stable main' \
        "$source_parts/caddy-stable.list" || exit 93
    grep -Fq 'URIs: https://archive.ubuntu.com/ubuntu https://custom.example/deb822' \
        "$source_parts/mixed.sources" || exit 94
    grep -Fq 'Signed-By: /custom-deb822.gpg' "$source_parts/mixed.sources" || exit 95
    if grep -Eq '^(deb|deb-src|URIs:).*dl\.cloudsmith\.io' "$source_list" "$source_parts"/*; then exit 96; fi
    if [ "$APT_SCENARIO" = mixed-error ]; then
        echo 'E: Failed to fetch https://archive.ubuntu.com/ubuntu/dists/noble/InRelease  Connection timed out'
        exit 100
    fi
    exit 0
fi
case "$APT_SCENARIO" in
    success) exit 0 ;;
    signature|mixed-signature)
        echo 'E: GPG error: https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version InRelease: signatures were invalid: BADSIG'
        if [ "$APT_SCENARIO" = mixed-signature ]; then
            echo 'E: Failed to fetch https://dl.cloudsmith.io/public/caddy/stable/deb/debian/dists/any-version/InRelease  402 Payment Required'
        fi
        exit 100 ;;
    other)
        echo 'E: Failed to fetch https://custom.example/apt/dists/stable/InRelease  402 Payment Required'
        exit 100 ;;
    *)
        echo 'E: Failed to fetch https://dl.cloudsmith.io/public/caddy/stable/deb/debian/dists/any-version/InRelease  402  Payment Required [IP: 192.0.2.1 443]'
        echo "E: The repository 'https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version InRelease' is no longer signed."
        exit 100 ;;
esac
SCRIPT
chmod +x "$TEST_DIR/bin/"*

reset_state() {
    rm -rf "$TEST_DIR/sources" "$TEST_DIR/expected" "$TEST_DIR/captured-view"
    mkdir -p "$TEST_DIR/sources/sources.list.d"
    : > "$TEST_DIR/calls"
    : > "$TEST_DIR/views"
    export APT_SCENARIO=success CADDY_USABLE=1
    cat > "$TEST_DIR/sources/sources.list" <<'SOURCES'
deb [signed-by=/ubuntu.gpg] https://archive.ubuntu.com/ubuntu noble main
deb [signed-by=/caddy.gpg] http://dl.cloudsmith.io/public/caddy/stable/deb/debian/ any-version main
# Documentation https://dl.cloudsmith.io/public/caddy/stable/deb/debian
SOURCES
    cat > "$TEST_DIR/sources/sources.list.d/caddy-stable.list" <<'SOURCES'
# Keep operator comments.
  deb [signed-by=/caddy.gpg arch=amd64] https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version main
deb-src https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version main
deb [signed-by=/custom.gpg] https://custom.example/apt stable main
SOURCES
    cat > "$TEST_DIR/sources/sources.list.d/mixed.sources" <<'SOURCES'
Types: deb deb-src
URIs: https://dl.cloudsmith.io/public/caddy/stable/deb/debian
Suites: any-version
Components: main
Signed-By: /caddy.gpg

Types: deb
URIs: https://dl.cloudsmith.io/public/caddy/stable/deb/debian
# A comment inside a continued URI field must not hide the other source.
 https://archive.ubuntu.com/ubuntu https://custom.example/deb822
Suites: noble
Components: main
Signed-By: /custom-deb822.gpg
SOURCES
    cp -a "$TEST_DIR/sources" "$TEST_DIR/expected"
}

assert_unchanged() {
    diff -r "$TEST_DIR/expected" "$TEST_DIR/sources" || fail 'host sources were modified'
    while IFS= read -r view; do
        [ ! -e "$view" ] || fail 'temporary sources survived'
    done < "$TEST_DIR/views"
}

# Healthy repositories use the real source configuration and never retry.
reset_state
host_apt_get update -qq > "$TEST_DIR/out" 2>&1 || fail 'healthy refresh failed'
[ "$(wc -l < "$TEST_DIR/calls")" -eq 1 ] || fail 'healthy refresh retried'
[ ! -d "$TEST_DIR/captured-view" ] || fail 'healthy sources were filtered'
assert_unchanged

# The observed Cloudsmith failure recovers with signed Ubuntu and custom sources.
reset_state
export APT_SCENARIO=caddy-402
host_apt_get update -qq > "$TEST_DIR/out" 2>&1 || fail 'installed Caddy did not recover'
[ "$(wc -l < "$TEST_DIR/calls")" -eq 2 ] || fail 'missing strict retry'
grep -Fq 'Host source files are unchanged' "$TEST_DIR/out" || fail 'recovery was not reported'
grep -Fq '# Keep operator comments.' "$TEST_DIR/captured-view/sources.list.d/caddy-stable.list" || fail 'operator comment lost'
assert_unchanged

# Missing/broken Caddy, genuine signature errors, and unrelated errors remain fatal.
for scenario in missing-caddy signature mixed-signature other mixed-error; do
    reset_state
    export APT_SCENARIO="$scenario"
    [ "$scenario" != missing-caddy ] || export CADDY_USABLE=0
    if host_apt_get update -qq > "$TEST_DIR/out" 2>&1; then
        fail "accepted $scenario failure"
    else
        status=$?
    fi
    [ "$status" -eq 100 ] || fail "$scenario changed the error code to $status"
    expected_calls=1
    [ "$scenario" != mixed-error ] || expected_calls=2
    [ "$(wc -l < "$TEST_DIR/calls")" -eq "$expected_calls" ] || fail "unexpected $scenario retry"
    assert_unchanged
done

# A 402 with an unrecognized source layout cannot silently drop repositories.
reset_state
export APT_SCENARIO=caddy-402
sed -i 's|dl.cloudsmith.io/public/caddy/stable/deb/debian|operator.example/caddy|g' \
    "$TEST_DIR/sources/sources.list" "$TEST_DIR/sources/sources.list.d/"*
rm -rf "$TEST_DIR/expected"
cp -a "$TEST_DIR/sources" "$TEST_DIR/expected"
if host_apt_get update -qq > "$TEST_DIR/out" 2>&1; then fail 'unknown source layout was accepted'; fi
[ "$(wc -l < "$TEST_DIR/calls")" -eq 1 ] || fail 'unknown layout retried'
assert_unchanged

# Package installation is passed through, and NodeSource's own refreshes use
# the same recovery after adding a new source. Its upstream script is unchanged.
reset_state
export APT_SCENARIO=caddy-402
cat > "$TEST_DIR/bin/curl" <<'SCRIPT'
#!/usr/bin/env bash
cat <<'SETUP'
set -euo pipefail
apt-get update -y
printf 'deb [signed-by=/node.gpg] https://deb.nodesource.com/node_22.x nodistro main\n' \
    > "$APT_TEST_DIR/sources/sources.list.d/nodesource.list"
apt-get update -y
SETUP
SCRIPT
chmod +x "$TEST_DIR/bin/curl"
install_nodesource_node 22 > "$TEST_DIR/out" 2>&1 || fail 'NodeSource refresh failed'
grep -Fq 'https://deb.nodesource.com/node_22.x' \
    "$TEST_DIR/captured-view/sources.list.d/nodesource.list" || fail 'new NodeSource repository was lost'
grep -Fxq 'install -y -qq nodejs' "$TEST_DIR/calls" || fail 'node package was not installed'
cp "$TEST_DIR/sources/sources.list.d/nodesource.list" "$TEST_DIR/expected/sources.list.d/"
assert_unchanged

echo 'PASS: host-apt'
