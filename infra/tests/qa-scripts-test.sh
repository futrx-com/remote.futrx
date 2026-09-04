#!/usr/bin/env bash
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
INSTALL_SCRIPT="$TESTS_DIR/../qa/install.sh"
UPDATE_SCRIPT="$TESTS_DIR/../qa/update.sh"
DEPLOY_APP_SCRIPT="$TESTS_DIR/../qa/deploy-app.sh"
DEPLOY_LOCAL_SCRIPT="$TESTS_DIR/../qa/deploy-local.sh"
COMMON_SCRIPT="$TESTS_DIR/../qa/common.sh"
CORE_INSTALL_SCRIPT="$TESTS_DIR/../install.sh"
HOST_DEPS_SCRIPT="$TESTS_DIR/../steps/01-host-deps.sh"
ROOT_PACKAGE_JSON="$TESTS_DIR/../../package.json"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

for script in "$COMMON_SCRIPT" "$INSTALL_SCRIPT" "$UPDATE_SCRIPT" "$DEPLOY_APP_SCRIPT" "$DEPLOY_LOCAL_SCRIPT"; do
    bash -n "$script"
done

bash "$INSTALL_SCRIPT" --help | grep -q 'public curl|bash command' || \
    fail "install help does not identify the public installer contract"
bash "$UPDATE_SCRIPT" --help | grep -q 'existing QA installation' || \
    fail "update help does not identify the existing-installation contract"
bash "$DEPLOY_APP_SCRIPT" --help | grep -q 'application code only' || \
    fail "deploy-app help does not identify the app-only contract"
bash "$DEPLOY_LOCAL_SCRIPT" --help | grep -q 'without a commit' || \
    fail "deploy-local help does not identify the local-working-tree contract"

if QA_ENV_FILE="$TEST_DIR/missing.env" bash "$INSTALL_SCRIPT" >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "install.sh accepted missing QA configuration"
fi
grep -q 'missing .*missing.env' "$TEST_DIR/err" || \
    fail "install.sh gave an unclear missing-config error"

if QA_ENV_FILE="$TEST_DIR/missing.env" bash "$INSTALL_SCRIPT" main >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "install.sh accepted missing QA configuration for a candidate install"
fi
grep -q 'missing .*missing.env' "$TEST_DIR/err" || \
    fail "install.sh gave an unclear candidate missing-config error"

if QA_ENV_FILE="$TEST_DIR/missing.env" bash "$INSTALL_SCRIPT" 'bad ref' >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "install.sh accepted an unsafe Git ref"
fi
grep -q 'unsupported characters' "$TEST_DIR/err" || \
    fail "install.sh gave an unclear unsafe-ref error"

if QA_ENV_FILE="$TEST_DIR/missing.env" bash "$UPDATE_SCRIPT" main >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "update.sh accepted missing QA configuration"
fi
grep -q 'missing .*missing.env' "$TEST_DIR/err" || \
    fail "update.sh gave an unclear missing-config error"

if QA_ENV_FILE="$TEST_DIR/missing.env" bash "$UPDATE_SCRIPT" 'bad ref' >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "update.sh accepted an unsafe Git ref"
fi
grep -q 'unsupported characters' "$TEST_DIR/err" || \
    fail "update.sh gave an unclear unsafe-ref error"

if QA_ENV_FILE="$TEST_DIR/missing.env" bash "$DEPLOY_APP_SCRIPT" main >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "deploy-app.sh accepted missing QA configuration"
fi
grep -q 'missing .*missing.env' "$TEST_DIR/err" || \
    fail "deploy-app.sh gave an unclear missing-config error"

if QA_ENV_FILE="$TEST_DIR/missing.env" bash "$DEPLOY_APP_SCRIPT" 'bad ref' >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "deploy-app.sh accepted an unsafe Git ref"
fi
grep -q 'unsupported characters' "$TEST_DIR/err" || \
    fail "deploy-app.sh gave an unclear unsafe-ref error"

if QA_ENV_FILE="$TEST_DIR/missing.env" bash "$DEPLOY_LOCAL_SCRIPT" >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "deploy-local.sh accepted missing QA configuration"
fi
grep -q 'missing .*missing.env' "$TEST_DIR/err" || \
    fail "deploy-local.sh gave an unclear missing-config error"

if bash "$CORE_INSTALL_SCRIPT" test.example.com --ref=main >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "core install.sh accepted a movable ref"
fi
grep -q '^--ref must be a full 40-character commit SHA$' "$TEST_DIR/err" || \
    fail "core install.sh gave an unclear immutable-ref error"

if FUTRX_INSTALL_DIR="$TEST_DIR/bootstrap-install" \
    bash -s -- test.example.com --ref=main <"$CORE_INSTALL_SCRIPT" \
    >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "curl-mode core install.sh accepted a movable ref"
fi
grep -q '^--ref must be a full 40-character commit SHA$' "$TEST_DIR/err" || \
    fail "curl-mode core install.sh gave an unclear immutable-ref error"

grep -Fq 'curl -fsSL "$install_url" | sudo bash -s -- "$public_host"' "$INSTALL_SCRIPT" || \
    fail "install.sh does not use the documented public curl pipeline"
grep -Fq 'raw.githubusercontent.com/futrx-com/remote.futrx/${CANDIDATE_SHA}/infra/install.sh' "$INSTALL_SCRIPT" || \
    fail "install.sh does not download the installer from the immutable candidate commit"
grep -Fq '"--ref=$candidate_sha"' "$INSTALL_SCRIPT" || \
    fail "install.sh does not pin the bootstrap clone to the candidate commit"
grep -Fq 'git clone --depth=1 --branch main --single-branch "$CLONE_URL" "$TARGET"' "$CORE_INSTALL_SCRIPT" || \
    fail "core install.sh does not pin production bootstrap clones to main"
grep -Fq 'MAIN_REFSPEC="+refs/heads/main:refs/remotes/origin/main"' "$CORE_INSTALL_SCRIPT" || \
    fail "core install.sh does not explicitly fetch main for existing narrow clones"
if [ "$(grep -Fc 'config remote.origin.fetch "$MAIN_REFSPEC"' "$CORE_INSTALL_SCRIPT")" -ne 2 ]; then
    fail "core install.sh does not persist the main refspec for both existing-checkout repair paths (legacy + primary), so later plain fetches would go stale"
fi
if grep -Eq 'apt-get|git clone' "$INSTALL_SCRIPT"; then
    fail "install.sh bootstraps dependencies or clones the repository itself"
fi
grep -Fq 'apt-get install -y -qq lxd lxd-client' "$HOST_DEPS_SCRIPT" || \
    fail "host dependencies do not use native Debian LXD inside nested LXC"
grep -Fq 'lxc profile set default security.idmap.base "$LXD_IDMAP_BASE"' "$HOST_DEPS_SCRIPT" || \
    fail "nested LXD does not preserve Remote's expected unprivileged idmap"
if grep -Eq 'infra/tests|npm --prefix|go (test|vet)' "$COMMON_SCRIPT"; then
    fail "QA install/update scripts run local project tests"
fi
for required_contract in 'npm run build' 'go build -trimpath' 'systemctl restart' 'wait_for_http_health' 'QA_DEPLOYED_SHA'; do
    grep -Fq "$required_contract" "$DEPLOY_APP_SCRIPT" || \
        fail "deploy-app.sh is missing contract: $required_contract"
done
if grep -Eq 'infra/(install|update|upgrade-workspaces)[.]sh|FORCE_REBUILD_BASE_IMAGE|apt-get' "$DEPLOY_APP_SCRIPT"; then
    fail "deploy-app.sh invokes host or workspace convergence"
fi
for required_contract in 'git ls-files' 'scp -q' 'npm run build' 'go build -trimpath' 'systemctl restart' 'wait_for_http_health' 'QA_DEPLOYED_VERSION'; do
    grep -Fq "$required_contract" "$DEPLOY_LOCAL_SCRIPT" || \
        fail "deploy-local.sh is missing contract: $required_contract"
done
if grep -Eq 'git (fetch|reset)|infra/(install|update|upgrade-workspaces)[.]sh|FORCE_REBUILD_BASE_IMAGE|apt-get' "$DEPLOY_LOCAL_SCRIPT"; then
    fail "deploy-local.sh changes the installed checkout, host, or workspaces"
fi
for package_contract in \
    '"qa:install": "QA_ENV_FILE=${QA_ENV_FILE:-./.qa.env} bash infra/qa/install.sh"' \
    '"qa:update": "QA_ENV_FILE=${QA_ENV_FILE:-./.qa.env} bash infra/qa/update.sh"' \
    '"qa:deploy-app": "QA_ENV_FILE=${QA_ENV_FILE:-./.qa.env} bash infra/qa/deploy-app.sh"' \
    '"qa:deploy-local": "QA_ENV_FILE=${QA_ENV_FILE:-./.qa.env} bash infra/qa/deploy-local.sh"' \
    '"qa:test": "bash infra/tests/qa-scripts-test.sh"'; do
    grep -Fq "$package_contract" "$ROOT_PACKAGE_JSON" || \
        fail "root package is missing QA command: $package_contract"
done

bash "$TESTS_DIR/qa-resolve-test.sh"
bash "$TESTS_DIR/lxd-host-test.sh"

echo "QA install/update/app-deploy/local-deploy script tests passed"
