#!/usr/bin/env bash
# QA application-only deploy wrapper for an existing QA installation.
#
# Unlike the production deployer, this accepts a pushed branch, tag, or commit
# and intentionally does not enforce a semantic-version release boundary. Use
# it when an immutable, shareable candidate needs frontend/backend deployment
# alone. Use deploy-local.sh for an uncommitted working tree, or update.sh when
# the candidate changes host or workspace infrastructure. Connection settings
# come from .qa.env (see .qa.env.example).

set -euo pipefail

QA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=common.sh
. "$QA_DIR/common.sh"

usage() {
    cat <<'EOF'
Usage: bash infra/qa/deploy-app.sh <branch|tag|commit>

Deploys application code only from one exact pushed commit: it builds the
frontend and backend on the existing QA server, restarts remote.futrx.service,
and verifies local and public health. It does not converge host dependencies,
Caddy, LXD, the workspace base image, or project containers. Use
infra/qa/deploy-local.sh to test an uncommitted working tree, or
infra/qa/update.sh to exercise the full production infrastructure path.

The requested ref must resolve to the clean local HEAD and to the same commit
on origin. After the binary is replaced, restart or health-check failure
restores the previous binary and checkout. A build failure leaves the live
binary untouched, although the QA checkout remains on the candidate commit.
EOF
}

case "${1:-}" in
    -h|--help) usage; exit 0 ;;
    '') usage >&2; exit 2 ;;
esac
[ "$#" -eq 1 ] || {
    usage >&2
    exit 2
}

qa_prepare "$1"

printf '==> Deploying QA application at %s on %s@%s\n' \
    "$QA_CANDIDATE_SHA" "$QA_SSH_USER" "$QA_SSH_HOST"
printf '    app only: host dependencies, base image, and workspaces are unchanged\n'

ssh "${QA_SSH_ARGS[@]}" "$QA_SSH_USER@$QA_SSH_HOST" \
    bash -s -- "$QA_REQUESTED_REF" "$QA_CANDIDATE_SHA" <<'REMOTE'
set -euo pipefail

requested_ref="$1"
candidate_sha="$2"
install_dir="/opt/remote.futrx"
service_name="remote.futrx.service"
binary="$install_dir/backend/remote"

if [ ! -d "$install_dir/.git" ] || ! systemctl cat "$service_name" >/dev/null 2>&1; then
    echo "QA server does not have an installed remote.futrx application" >&2
    echo "Use infra/qa/install.sh before deploying application code." >&2
    exit 3
fi
for command_name in git npm go systemctl; do
    command -v "$command_name" >/dev/null 2>&1 || {
        echo "QA server is missing required command: $command_name" >&2
        echo "Use infra/qa/update.sh to converge host dependencies." >&2
        exit 1
    }
done
[ -x "$binary" ] || {
    echo "installed application binary is missing or not executable: $binary" >&2
    exit 1
}

cd "$install_dir"
previous_sha="$(git rev-parse --verify 'HEAD^{commit}')"
git fetch --quiet origin "$requested_ref"
remote_sha="$(git rev-parse --verify 'FETCH_HEAD^{commit}')"
if [ "$remote_sha" != "$candidate_sha" ]; then
    echo "remote fetched $remote_sha, expected $candidate_sha" >&2
    exit 1
fi
git reset --hard "$candidate_sha"

stage_dir="$(mktemp -d "$install_dir/backend/.qa-app-deploy.XXXXXX")"
cleanup() {
    rm -f "$stage_dir/remote" "$stage_dir/previous"
    rmdir "$stage_dir" 2>/dev/null || true
}
trap cleanup EXIT
cp -p "$binary" "$stage_dir/previous"

echo "==> Building frontend"
(
    cd frontend
    npm install --silent --no-audit --no-fund 2>&1 | tail -3
    npm run build
)

echo "==> Building backend"
app_version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
(
    cd backend
    go build -trimpath \
        -ldflags="-s -w -X github.com/futrx-com/remote.futrx.com/internal/version.Version=${app_version}" \
        -o "$stage_dir/remote" ./cmd/remote
)

rollback() {
    echo "App deployment failed; restoring the previous application binary" >&2
    install -m 0755 "$stage_dir/previous" "$binary"
    git reset --hard "$previous_sha" >/dev/null 2>&1 || true
    systemctl restart "$service_name" || true
}

install -m 0755 "$stage_dir/remote" "$binary"
if ! systemctl restart "$service_name"; then
    rollback
    exit 1
fi

# shellcheck source=../lib/health-check.sh
. infra/lib/health-check.sh
if ! wait_for_http_health http://127.0.0.1:7682/ 30; then
    rollback
    exit 1
fi

deployed_sha="$(git rev-parse --verify 'HEAD^{commit}')"
if [ "$deployed_sha" != "$candidate_sha" ]; then
    rollback
    echo "deployed $deployed_sha, expected candidate $candidate_sha" >&2
    exit 1
fi
systemctl is-active --quiet "$service_name"
printf 'QA_DEPLOYED_SHA=%s\n' "$deployed_sha"
REMOTE

qa_verify_public_url

printf '\n✓ QA application deployed at %s\n' "$QA_CANDIDATE_SHA"
printf '  https://%s/\n' "$QA_PUBLIC_HOST"
