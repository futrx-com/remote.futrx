#!/usr/bin/env bash
# QA wrapper for a full infrastructure update of an existing installation.
#
# It resolves a pushed branch, tag, or commit to an immutable SHA, runs the
# production infra/update.sh on the QA host, and verifies local and public
# health. This is disruptive and can rebuild the base image and recycle
# eligible project containers. Use infra/qa/deploy-app.sh for an app-only
# pushed candidate or infra/qa/deploy-local.sh for an uncommitted app-only
# candidate. Connection settings come from .qa.env (see .qa.env.example).

set -euo pipefail

QA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=common.sh
. "$QA_DIR/common.sh"

usage() {
    cat <<'EOF'
Usage: bash infra/qa/update.sh <branch|tag|commit>

Updates an existing QA installation to an exact pushed Git commit. It refuses
to provision a fresh server; use infra/qa/install.sh for the first install. It
runs the complete production infrastructure path, including host convergence,
base-image rebuild, and eligible workspace recycling.

The requested ref must resolve to the clean local HEAD and to the same commit
on origin. Coordinate a maintenance window before running it.
EOF
}

case "${1:-}" in
    -h|--help) usage; exit 0 ;;
    '') usage >&2; exit 2 ;;
esac

qa_prepare "$1"

printf '==> Updating QA to %s on %s@%s\n' \
    "$QA_CANDIDATE_SHA" "$QA_SSH_USER" "$QA_SSH_HOST"

ssh "${QA_SSH_ARGS[@]}" "$QA_SSH_USER@$QA_SSH_HOST" \
    bash -s -- "$QA_REQUESTED_REF" "$QA_CANDIDATE_SHA" <<'REMOTE'
set -euo pipefail

requested_ref="$1"
candidate_sha="$2"
install_dir="/opt/remote.futrx"

if [ ! -d "$install_dir/.git" ]; then
    echo "QA server is fresh: remote.futrx is not installed at $install_dir" >&2
    echo "Use infra/qa/install.sh before testing updates." >&2
    exit 3
fi

cd "$install_dir"
git fetch --quiet origin "$requested_ref"
remote_sha="$(git rev-parse --verify 'FETCH_HEAD^{commit}')"
if [ "$remote_sha" != "$candidate_sha" ]; then
    echo "remote fetched $remote_sha, expected $candidate_sha" >&2
    exit 1
fi

# Fetching above places the exact object in this checkout, allowing update.sh
# to pin the deployment by immutable SHA even when the source was a branch.
bash infra/update.sh "--ref=$candidate_sha"

deployed_sha="$(git rev-parse --verify 'HEAD^{commit}')"
if [ "$deployed_sha" != "$candidate_sha" ]; then
    echo "updated $deployed_sha, expected $candidate_sha" >&2
    exit 1
fi

systemctl is-active --quiet remote.futrx
. infra/lib/health-check.sh
wait_for_http_health http://127.0.0.1:7682/ 30
printf 'QA_UPDATED_SHA=%s\n' "$deployed_sha"
REMOTE

qa_verify_public_url

printf '\n✓ QA updated to %s\n' "$QA_CANDIDATE_SHA"
printf '  https://%s/\n' "$QA_PUBLIC_HOST"
