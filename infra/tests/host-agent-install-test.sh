#!/usr/bin/env bash
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_ROOT="$(cd "$TESTS_DIR/../.." >/dev/null 2>&1 && pwd)"
INSTALLER="$TESTS_DIR/../install.sh"
CHECKOUT_STEP="$TESTS_DIR/../steps/00-checkout.sh"
HOST_DEPS="$TESTS_DIR/../steps/01-host-deps.sh"
APP_STEP="$TESTS_DIR/../steps/02-app.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

bash -n "$INSTALLER"
bash -n "$CHECKOUT_STEP"
bash -n "$HOST_DEPS"
bash -n "$APP_STEP"
grep -Fq 'go run ./cmd/install-host-agents' "$APP_STEP" || \
    fail "application step does not invoke the module-driven host installer"
if grep -Fq 'go run ./cmd/install-host-agents' "$HOST_DEPS"; then
    fail "host installer runs before the selected application checkout is available"
fi
checkout_source_line="$(grep -n 'steps/00-checkout.sh' "$INSTALLER" | tail -1 | cut -d: -f1)"
host_deps_source_line="$(grep -n 'steps/01-host-deps.sh' "$INSTALLER" | tail -1 | cut -d: -f1)"
app_source_line="$(grep -n 'steps/02-app.sh' "$INSTALLER" | tail -1 | cut -d: -f1)"
if [ -z "$checkout_source_line" ] || [ -z "$host_deps_source_line" ] || [ -z "$app_source_line" ] || \
   [ "$checkout_source_line" -ge "$host_deps_source_line" ] || \
   [ "$host_deps_source_line" -ge "$app_source_line" ]; then
    fail "target checkout must be selected before host dependencies and agent convergence"
fi
grep -Fq 'exec bash "$INSTALL_DIR/infra/install.sh" "$@"' "$CHECKOUT_STEP" || \
    fail "checkout selection does not re-execute the selected installer"

for stale_contract in \
    'ensure_agent_cli' \
    '@anthropic-ai/claude-code' \
    '@openai/codex' \
    '@moonshot-ai/kimi-code' \
    'CLAUDE_CODE_VERSION CODEX_CLI_VERSION KIMI_CODE_VERSION'; do
    if grep -Fq "$stale_contract" "$HOST_DEPS" || grep -Fq "$stale_contract" "$APP_STEP"; then
        fail "infrastructure steps still hardcode agent policy: $stale_contract"
    fi
done

# shellcheck source=../versions.env
. "$TESTS_DIR/../versions.env"
plan="$(cd "$REPO_ROOT/backend" && go run ./cmd/install-host-agents -plan)"
grep -Fxq $'claude\tclaude\t'"$CLAUDE_CODE_VERSION"$'\tnpm\t@anthropic-ai/claude-code' <<<"$plan" || \
    fail "host plan is missing Claude"
grep -Fxq $'codex\tcodex\t'"$CODEX_CLI_VERSION"$'\tnpm\t@openai/codex' <<<"$plan" || \
    fail "host plan is missing Codex"
grep -Fxq $'kimi\tkimi\t'"$KIMI_CODE_VERSION"$'\timage-repair\t@moonshot-ai/kimi-code' <<<"$plan" || \
    fail "host plan is missing Kimi"
grep -Fxq $'antigravity\tagy\t'"$ANTIGRAVITY_CLI_VERSION"$'\tscript\t-' <<<"$plan" || \
    fail "host plan is missing Antigravity"

echo "host agent install tests passed"
