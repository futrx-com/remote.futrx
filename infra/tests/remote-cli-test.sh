#!/usr/bin/env bash
set -euo pipefail

INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT
CLI_PATH="$TEST_DIR/bin/remote"
mkdir -p "$TEST_DIR/bin" "$TEST_DIR/elsewhere"

fail() { echo "FAIL: $*" >&2; exit 1; }

install_cli() (
    log() { :; }
    # Stop before the step starts configuring systemd and the login profile.
    render_template() { exit 0; }
    install() {
        local options=()
        while [ "$#" -gt 2 ]; do
            case "$1" in
                -o|-g) shift 2 ;; # The test also runs as an unprivileged user.
                *) options+=("$1"); shift ;;
            esac
        done
        [ "$2" = /usr/local/bin/remote ] || fail "unexpected install destination: $2"
        command install "${options[@]}" "$1" "$CLI_PATH"
    }
    . "$INFRA_DIR/steps/04-backend-svc.sh"
)

# Exercise first installation and reinstallation with different settings.
for name in normal 'custom path '\''$(exit 99)'; do
    INSTALL_DIR="$TEST_DIR/$name"
    HOSTNAME="remote.example.com"
    [ "$name" = normal ] || HOSTNAME="updated.example.com"
    mkdir -p "$INSTALL_DIR/backend"
    cat > "$INSTALL_DIR/backend/remote" <<'EOF'
#!/bin/bash
printf '%s\0' "$BASE_URL" "$DATA_DIR" "$INSTALL_DIR" "$@"
printf 'backend stderr\n' >&2
exit 23
EOF
    chmod 0755 "$INSTALL_DIR/backend/remote"
    install_cli
    [ "$(stat -c '%a' "$CLI_PATH")" = 755 ] || fail "launcher is not mode 0755"
    bash -n "$CLI_PATH"

    # Resolve through PATH from another directory, overriding stale caller env.
    status=0
    (
        cd "$TEST_DIR/elsewhere"
        PATH="$TEST_DIR/bin:$PATH" BASE_URL=wrong DATA_DIR=wrong INSTALL_DIR=wrong \
            remote setup-token 'argument with spaces' '$(exit 99)'
    ) > "$TEST_DIR/actual" 2> "$TEST_DIR/stderr" || status=$?
    [ "$status" = 23 ] || fail "backend exit status was not forwarded"
    printf '%s\0' "https://$HOSTNAME" "$INSTALL_DIR/data" "$INSTALL_DIR" \
        setup-token 'argument with spaces' '$(exit 99)' > "$TEST_DIR/expected"
    cmp "$TEST_DIR/expected" "$TEST_DIR/actual" || fail "incorrect configuration or arguments"
    [ "$(cat "$TEST_DIR/stderr")" = 'backend stderr' ] || fail "stderr was not forwarded"
done

echo "remote CLI tests passed"
