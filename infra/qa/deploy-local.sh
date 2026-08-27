#!/usr/bin/env bash
# QA application-only deploy from the current local working tree.
#
# This command does not require a commit or push. It packages tracked files
# plus non-ignored untracked files, uploads them to a temporary directory on
# the QA server, builds the frontend and backend there, and replaces only the
# installed application binary. The server's /opt/remote.futrx Git checkout is
# left unchanged.
#
# Use this only for frontend/backend iteration. Local infrastructure changes
# are present in the uploaded build context but are not applied to the host,
# base image, or project containers. Exercise those changes through the
# immutable, pushed-ref path in infra/qa/update.sh.
#
# Connection settings come from .qa.env (see .qa.env.example). The QA account
# must be able to write the installed binary and restart remote.futrx.service.

set -euo pipefail

QA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=common.sh
. "$QA_DIR/common.sh"

usage() {
    cat <<'EOF'
Usage: bash infra/qa/deploy-local.sh

Deploys application code from the current local working tree without a commit
or push. Tracked files and non-ignored untracked files are built in a temporary
directory on the QA server. The installed Git checkout is not changed.

The deployment rebuilds the frontend and backend, atomically replaces the
application binary, restarts remote.futrx.service, and verifies local and
public health. Restart or local health-check failure restores the previous
binary. Host dependencies, Caddy, LXD, the base image, and project containers
are not changed.
EOF
}

case "${1:-}" in
    -h|--help) usage; exit 0 ;;
    '') ;;
    *) usage >&2; exit 2 ;;
esac
[ "$#" -eq 0 ] || {
    usage >&2
    exit 2
}

qa_prepare_connection

for command_name in git tar scp ssh; do
    command -v "$command_name" >/dev/null 2>&1 || \
        qa_fail "required local command is missing: $command_name"
done

cd "$QA_REPO_ROOT"

head_sha="$(git rev-parse --verify HEAD)"
short_sha="$(git rev-parse --short=12 HEAD)"
tree_state="clean"
if [ -n "$(git status --porcelain --untracked-files=normal)" ]; then
    tree_state="dirty"
fi
local_version="qa-local-${short_sha}-${tree_state}-$(date -u +%Y%m%d%H%M%S)"

package_dir="$(mktemp -d)"
archive_path="$package_dir/source.tar.gz"
manifest_path="$package_dir/files.list"
remote_archive=""

cleanup() {
    rm -rf -- "$package_dir"
    if [ -n "$remote_archive" ]; then
        ssh "${QA_SSH_ARGS[@]}" "$QA_SSH_USER@$QA_SSH_HOST" \
            bash -s -- "$remote_archive" >/dev/null 2>&1 <<'REMOTE_CLEANUP' || true
rm -f -- "$1"
REMOTE_CLEANUP
    fi
}
trap cleanup EXIT

# git ls-files includes tracked modifications and non-ignored untracked files,
# but also reports tracked files deleted from the working tree. Build a null-
# delimited manifest containing only files that are actually present.
while IFS= read -r -d '' relative_path; do
    if [ -f "$QA_REPO_ROOT/$relative_path" ] || [ -L "$QA_REPO_ROOT/$relative_path" ]; then
        printf '%s\0' "$relative_path"
    fi
done < <(git ls-files --cached --others --exclude-standard -z) > "$manifest_path"

[ -s "$manifest_path" ] || qa_fail "the local working tree contains no deployable files"

tar --create --gzip \
    --file "$archive_path" \
    --directory "$QA_REPO_ROOT" \
    --null \
    --files-from "$manifest_path"

printf '==> Packaging local QA application at %s (%s)\n' "$head_sha" "$tree_state"
printf '    version: %s\n' "$local_version"
printf '    app only: the installed Git checkout and infrastructure stay unchanged\n'

remote_archive="$(ssh "${QA_SSH_ARGS[@]}" "$QA_SSH_USER@$QA_SSH_HOST" \
    mktemp '/tmp/remote-futrx-local.XXXXXX.tar.gz')"
case "$remote_archive" in
    /tmp/remote-futrx-local.*.tar.gz) ;;
    *) qa_fail "QA server returned an unexpected temporary path: $remote_archive" ;;
esac

scp -q "${QA_SSH_ARGS[@]}" "$archive_path" \
    "$QA_SSH_USER@$QA_SSH_HOST:$remote_archive"

printf '==> Building and deploying on %s@%s\n' "$QA_SSH_USER" "$QA_SSH_HOST"

ssh "${QA_SSH_ARGS[@]}" "$QA_SSH_USER@$QA_SSH_HOST" \
    bash -s -- "$remote_archive" "$local_version" <<'REMOTE'
set -euo pipefail

archive_path="$1"
app_version="$2"
install_dir="/opt/remote.futrx"
service_name="remote.futrx.service"
binary="$install_dir/backend/remote"
stage_dir=""
candidate_binary=""
previous_binary=""

cleanup_remote() {
    rm -f -- "$archive_path"
    [ -z "$candidate_binary" ] || rm -f -- "$candidate_binary"
    [ -z "$previous_binary" ] || rm -f -- "$previous_binary"
    [ -z "$stage_dir" ] || rm -rf -- "$stage_dir"
}
trap cleanup_remote EXIT

if [ ! -d "$install_dir/.git" ] || ! systemctl cat "$service_name" >/dev/null 2>&1; then
    echo "QA server does not have an installed remote.futrx application" >&2
    echo "Use infra/qa/install.sh before deploying local application code." >&2
    exit 3
fi
for command_name in tar npm go install mktemp mv systemctl; do
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
[ -r "$install_dir/infra/lib/health-check.sh" ] || {
    echo "installed health-check helper is missing" >&2
    echo "Use infra/qa/update.sh to converge the QA installation." >&2
    exit 1
}

stage_dir="$(mktemp -d /tmp/remote-futrx-local-build.XXXXXX)"
tar --extract --gzip --file "$archive_path" --directory "$stage_dir"
for required_path in frontend/package.json backend/go.mod backend/cmd/remote; do
    [ -e "$stage_dir/$required_path" ] || {
        echo "local source archive is missing: $required_path" >&2
        exit 1
    }
done

echo "==> Building frontend"
(
    cd "$stage_dir/frontend"
    npm install --silent --no-audit --no-fund 2>&1 | tail -3
    npm run build
)

echo "==> Building backend"
(
    cd "$stage_dir/backend"
    go build -trimpath \
        -ldflags="-s -w -X github.com/futrx-com/remote.futrx.com/internal/version.Version=${app_version}" \
        -o "$stage_dir/remote" ./cmd/remote
)

candidate_binary="$(mktemp "$install_dir/backend/.qa-local-candidate.XXXXXX")"
previous_binary="$(mktemp "$install_dir/backend/.qa-local-previous.XXXXXX")"
cp -p "$binary" "$previous_binary"
install -m 0755 "$stage_dir/remote" "$candidate_binary"

rollback() {
    echo "Local QA deployment failed; restoring the previous application binary" >&2
    install -m 0755 "$previous_binary" "$candidate_binary"
    mv -f "$candidate_binary" "$binary"
    systemctl restart "$service_name" || true
}

mv -f "$candidate_binary" "$binary"
if ! systemctl restart "$service_name"; then
    rollback
    exit 1
fi

# Use the installed health-check implementation because this app-only command
# must not apply or depend on local infrastructure changes.
# shellcheck source=../lib/health-check.sh
. "$install_dir/infra/lib/health-check.sh"
if ! wait_for_http_health http://127.0.0.1:7682/ 30; then
    rollback
    exit 1
fi
if ! systemctl is-active --quiet "$service_name"; then
    rollback
    exit 1
fi

printf 'QA_DEPLOYED_VERSION=%s\n' "$app_version"
REMOTE

remote_archive=""
qa_verify_public_url

printf '\n✓ Local QA application deployed\n'
printf '  Version: %s\n' "$local_version"
printf '  Source HEAD: %s (%s working tree)\n' "$head_sha" "$tree_state"
printf '  Git checkout: unchanged\n'
printf '  https://%s/\n' "$QA_PUBLIC_HOST"
