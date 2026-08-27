#!/usr/bin/env bash
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEPLOY_SCRIPT="$TESTS_DIR/../deploy-app.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

bash -n "$DEPLOY_SCRIPT"
bash "$DEPLOY_SCRIPT" --help | grep -q 'deploy frontend/backend code' || \
    fail "help does not identify the application-only contract"

if FUTRX_INSTALL_DIR="$TEST_DIR/missing" bash "$DEPLOY_SCRIPT" --ref=0.3.2 \
    >"$TEST_DIR/out" 2>"$TEST_DIR/err"; then
    fail "deploy-app.sh accepted a missing installation"
fi
grep -q 'not an installed git checkout' "$TEST_DIR/err" || \
    fail "missing-installation error is unclear"

for required_contract in \
    'release_update_kind' \
    'npm run build' \
    'go build -trimpath' \
    'systemctl restart' \
    'wait_for_http_health' \
    'restoring the previous release'; do
    grep -Fq "$required_contract" "$DEPLOY_SCRIPT" || \
        fail "deploy-app.sh is missing contract: $required_contract"
done

if grep -Eq '^[[:space:]]*(sudo[[:space:]]+)?bash .*infra/(install|update|upgrade-workspaces)[.]sh|FORCE_REBUILD_BASE_IMAGE|apt-get' "$DEPLOY_SCRIPT"; then
    fail "deploy-app.sh invokes host or workspace convergence"
fi

ORIGIN="$TEST_DIR/origin.git"
INSTALL_DIR="$TEST_DIR/install"
FAKE_BIN="$TEST_DIR/bin"
mkdir -p "$FAKE_BIN"
git init --bare --quiet "$ORIGIN"
git init --quiet "$INSTALL_DIR"
git -C "$INSTALL_DIR" config user.name Test
git -C "$INSTALL_DIR" config user.email test@example.com
git -C "$INSTALL_DIR" remote add origin "$ORIGIN"
mkdir -p "$INSTALL_DIR/frontend" "$INSTALL_DIR/backend" "$INSTALL_DIR/infra/lib"
printf '/backend/remote\n/backend/public/\n' > "$INSTALL_DIR/.gitignore"
printf 'baseline\n' > "$INSTALL_DIR/frontend/source.txt"
cat > "$INSTALL_DIR/infra/lib/health-check.sh" <<'EOF'
wait_for_http_health() {
    [ "${FAIL_HEALTH:-0}" != "1" ]
}
EOF
git -C "$INSTALL_DIR" add .
git -C "$INSTALL_DIR" commit --quiet -m baseline
git -C "$INSTALL_DIR" tag 0.4.0
printf 'patch\n' > "$INSTALL_DIR/frontend/source.txt"
git -C "$INSTALL_DIR" commit --quiet -am patch
git -C "$INSTALL_DIR" tag 0.4.1
printf 'minor\n' > "$INSTALL_DIR/frontend/source.txt"
git -C "$INSTALL_DIR" commit --quiet -am minor
git -C "$INSTALL_DIR" tag 0.5.0
git -C "$INSTALL_DIR" push --quiet origin HEAD --tags

cat > "$FAKE_BIN/npm" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$FAKE_BIN/go" <<'EOF'
#!/usr/bin/env bash
while [ "$#" -gt 0 ]; do
    if [ "$1" = "-o" ]; then
        printf 'new binary\n' > "$2"
        chmod 0755 "$2"
        exit 0
    fi
    shift
done
exit 1
EOF
cat > "$FAKE_BIN/systemctl" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$FAKE_BIN/npm" "$FAKE_BIN/go" "$FAKE_BIN/systemctl"

reset_install() {
    git -C "$INSTALL_DIR" reset --hard --quiet 0.4.0
    printf 'old binary\n' > "$INSTALL_DIR/backend/remote"
    chmod 0755 "$INSTALL_DIR/backend/remote"
}

reset_install
PATH="$FAKE_BIN:$PATH" FUTRX_INSTALL_DIR="$INSTALL_DIR" \
    bash "$DEPLOY_SCRIPT" --ref=0.4.1 >"$TEST_DIR/success.out"
[ "$(git -C "$INSTALL_DIR" rev-parse HEAD)" = "$(git -C "$INSTALL_DIR" rev-parse 0.4.1)" ] || \
    fail "successful patch deployment did not select its target commit"
grep -q '^new binary$' "$INSTALL_DIR/backend/remote" || \
    fail "successful patch deployment did not install the staged binary"

reset_install
if PATH="$FAKE_BIN:$PATH" FUTRX_INSTALL_DIR="$INSTALL_DIR" FAIL_HEALTH=1 \
    bash "$DEPLOY_SCRIPT" --ref=0.4.1 >"$TEST_DIR/failure.out" 2>"$TEST_DIR/failure.err"; then
    fail "deploy-app.sh accepted a failed health check"
fi
[ "$(git -C "$INSTALL_DIR" rev-parse HEAD)" = "$(git -C "$INSTALL_DIR" rev-parse 0.4.0)" ] || \
    fail "failed deployment did not restore the previous checkout"
grep -q '^old binary$' "$INSTALL_DIR/backend/remote" || \
    fail "failed deployment did not restore the previous binary"

reset_install
if PATH="$FAKE_BIN:$PATH" FUTRX_INSTALL_DIR="$INSTALL_DIR" \
    bash "$DEPLOY_SCRIPT" --ref=0.5.0 >"$TEST_DIR/minor.out" 2>"$TEST_DIR/minor.err"; then
    fail "deploy-app.sh accepted a cross-minor deployment"
fi
grep -q 'refusing application-only deployment' "$TEST_DIR/minor.err" || \
    fail "cross-minor refusal is unclear"
[ "$(git -C "$INSTALL_DIR" rev-parse HEAD)" = "$(git -C "$INSTALL_DIR" rev-parse 0.4.0)" ] || \
    fail "cross-minor refusal changed the installed checkout"

echo "Production application deploy script tests passed"
