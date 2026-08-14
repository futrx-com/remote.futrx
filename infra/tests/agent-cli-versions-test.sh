#!/usr/bin/env bash
set -euo pipefail

TEST_DIR="$(mktemp -d)"
trap 'command rm -rf -- "$TEST_DIR"' EXIT

# shellcheck source=../lib/agent-cli-versions.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib/agent-cli-versions.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

MANIFEST="$TEST_DIR/versions.env"
LINK="$TEST_DIR/infra-versions.env"
cat > "$MANIFEST" <<'EOF'
# unrelated content and ordering must survive
CLAUDE_CODE_VERSION=1.0.0
CODEX_CLI_VERSION=1.0.0
KIMI_CODE_VERSION=1.0.0
ANTIGRAVITY_CLI_VERSION=1.0.0
NODE_MAJOR=22
EOF
ln -s versions.env "$LINK"

write_agent_cli_versions "$LINK" 2.3.4 3.4.5 4.5.6 5.6.7-beta.1

[ -L "$LINK" ] || fail "manifest symlink was replaced"
[ "$(grep '^CLAUDE_CODE_VERSION=' "$MANIFEST")" = "CLAUDE_CODE_VERSION=2.3.4" ] || \
    fail "Claude version was not updated"
[ "$(grep '^CODEX_CLI_VERSION=' "$MANIFEST")" = "CODEX_CLI_VERSION=3.4.5" ] || \
    fail "Codex version was not updated"
[ "$(grep '^KIMI_CODE_VERSION=' "$MANIFEST")" = "KIMI_CODE_VERSION=4.5.6" ] || \
    fail "Kimi version was not updated"
[ "$(grep '^ANTIGRAVITY_CLI_VERSION=' "$MANIFEST")" = \
    "ANTIGRAVITY_CLI_VERSION=5.6.7-beta.1" ] || fail "Antigravity version was not updated"
[ "$(grep '^NODE_MAJOR=' "$MANIFEST")" = "NODE_MAJOR=22" ] || \
    fail "unrelated manifest values changed"

if write_agent_cli_versions "$LINK" invalid 3.4.5 4.5.6 5.6.7 2>/dev/null; then
    fail "invalid semantic version was accepted"
fi

MISSING="$TEST_DIR/missing.env"
grep -v '^CODEX_CLI_VERSION=' "$MANIFEST" > "$MISSING"
if write_agent_cli_versions "$MISSING" 2.3.4 3.4.5 4.5.6 5.6.7 2>/dev/null; then
    fail "manifest with a missing CLI key was accepted"
fi

printf '#!/usr/bin/env bash\nprintf "claude version 9.8.7 (build 123)\\n"\n' \
    > "$TEST_DIR/fake-cli"
chmod +x "$TEST_DIR/fake-cli"
[ "$(agent_cli_semver "$TEST_DIR/fake-cli")" = "9.8.7" ] || \
    fail "CLI version output was not parsed"

npm() {
    [ "$*" = "view @example/agent version" ] || fail "unexpected npm arguments: $*"
    printf '10.11.12\n'
}
[ "$(latest_npm_package_version @example/agent)" = "10.11.12" ] || \
    fail "latest npm package version was not resolved"
unset -f npm

ANTIGRAVITY_MANIFEST_URL=""
uname() {
    printf 'x86_64\n'
}
curl() {
    ANTIGRAVITY_MANIFEST_URL="${!#}"
    [ "$ANTIGRAVITY_MANIFEST_URL" = \
        "https://antigravity-cli-auto-updater-974169037036.us-central1.run.app/manifests/linux_amd64.json" ] || \
        fail "unexpected Antigravity manifest URL: $ANTIGRAVITY_MANIFEST_URL"
    printf '{"version":"13.14.15"}\n'
}
jq() {
    sed -n 's/.*"version":"\([^"]*\)".*/\1/p'
}
[ "$(latest_antigravity_cli_version)" = "13.14.15" ] || \
    fail "latest Antigravity version was not resolved"
unset -f uname curl jq

echo "PASS: agent CLI version manifest tests"
