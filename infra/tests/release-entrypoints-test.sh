#!/usr/bin/env bash
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
UPDATE_SCRIPT="$TESTS_DIR/../update.sh"
INSTALL_SCRIPT="$TESTS_DIR/../install.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

bash -n "$UPDATE_SCRIPT"
bash -n "$INSTALL_SCRIPT"

grep -Fq '    remote_refresh_checkout "$@"' "$UPDATE_SCRIPT" || \
    fail "updater main does not forward arguments to checkout refresh"
grep -Fq '    remote_select_checkout "$@"' "$INSTALL_SCRIPT" || \
    fail "installer main does not forward arguments to checkout selection"
grep -Fq 'step_00_checkout "$@"' "$INSTALL_SCRIPT" || \
    fail "installer checkout wrapper does not forward arguments to the step"
grep -Fq 'remote_exec_selected_installer "$INSTALL_DIR/infra/install.sh" "$@"' \
    "$TESTS_DIR/../steps/00-checkout.sh" || \
    fail "installer checkout step bypasses protected credential forwarding"

# Production checkout selection must deploy a numeric release, not an
# untagged main commit. When tags share a commit, the highest selected tag is
# carried through re-exec so the backend can stamp it exactly.
ORIGIN="$TEST_DIR/release-origin.git"
SOURCE="$TEST_DIR/release-source"
SELECTED_INSTALL="$TEST_DIR/release-install"
git init --bare --quiet "$ORIGIN"
git init --quiet "$SOURCE"
git -C "$SOURCE" config user.name Test
git -C "$SOURCE" config user.email test@example.com
printf 'release\n' > "$SOURCE/source.txt"
git -C "$SOURCE" add source.txt
git -C "$SOURCE" commit --quiet -m release
release_commit="$(git -C "$SOURCE" rev-parse HEAD)"
git -C "$SOURCE" tag 0.21.0
git -C "$SOURCE" tag 0.21.1
printf 'unreleased\n' >> "$SOURCE/source.txt"
git -C "$SOURCE" commit --quiet -am unreleased
git -C "$SOURCE" branch -M main
git -C "$SOURCE" remote add origin "$ORIGIN"
git -C "$SOURCE" push --quiet origin main --tags
git --git-dir="$ORIGIN" symbolic-ref HEAD refs/heads/main
(
    unset __FUTRX_DEFAULTS_SOURCED FUTRX_INSTALL_CHECKOUT_SELECTED
    unset FUTRX_CHECKOUT_REF GITHUB_TOKEN GOOGLE_CLIENT_SECRET
    # shellcheck source=../install.sh
    . "$INSTALL_SCRIPT"
    remote_load_configuration
    INSTALL_DIR="$SELECTED_INSTALL"
    REPO_URL="$ORIGIN"
    selected_state="$TEST_DIR/release-selected-state"
    remote_replace_process() {
        printf '%s\n%s\n' \
            "$FUTRX_CHECKOUT_REF" \
            "$(git -C "$INSTALL_DIR" rev-parse HEAD)" > "$selected_state"
    }

    remote_select_checkout remote.example.com --skip-dns-check
)
mapfile -t selected_state < "$TEST_DIR/release-selected-state"
[ "${selected_state[0]}" = "0.21.1" ] ||
    fail "production checkout did not select the highest release tag"
[ "${selected_state[1]}" = "$release_commit" ] ||
    fail "production checkout followed untagged main instead of the release"

# A full update without --ref follows the same release-only policy and carries
# the resolved tag through its self re-exec.
(
    unset __FUTRX_DEFAULTS_SOURCED FUTRX_UPDATE_REEXECED
    unset FUTRX_UPDATE_SELECTED_REF FUTRX_CHECKOUT_REF
    # shellcheck source=../update.sh
    . "$UPDATE_SCRIPT"
    remote_load_configuration
    INSTALL_DIR="$SELECTED_INSTALL"
    git -C "$INSTALL_DIR" reset --hard origin/main >/dev/null
    if git -C "$INSTALL_DIR" describe --exact-match --tags HEAD >/dev/null 2>&1; then
        fail "legacy updater fixture is not on untagged main"
    fi
    remote_parse_update_arguments --skip-workspaces
    update_state="$TEST_DIR/update-selected-state"
    exec() {
        printf '%s\n%s\n' \
            "$FUTRX_UPDATE_SELECTED_REF" \
            "$(git -C "$INSTALL_DIR" rev-parse HEAD)" > "$update_state"
    }

    remote_refresh_checkout --skip-workspaces
    TARGET_REF=""
    remote_parse_update_arguments --skip-workspaces
    printf '%s\n' "$TARGET_REF" >> "$update_state"
)
mapfile -t update_state < "$TEST_DIR/update-selected-state"
[ "${update_state[0]}" = "0.21.1" ] ||
    fail "no-ref infrastructure update did not select the highest release"
[ "${update_state[1]}" = "$release_commit" ] ||
    fail "no-ref infrastructure update followed untagged main"
[ "${update_state[2]}" = "0.21.1" ] ||
    fail "no-ref infrastructure update lost its selected release during re-exec"

# A retracted release must not remain eligible merely because an installed
# checkout fetched it previously. Production selection follows the current
# remote tag set.
git -C "$SOURCE" push --quiet origin :refs/tags/0.21.1
(
    unset __FUTRX_DEFAULTS_SOURCED FUTRX_INSTALL_CHECKOUT_SELECTED
    unset FUTRX_CHECKOUT_REF GITHUB_TOKEN GOOGLE_CLIENT_SECRET
    # shellcheck source=../install.sh
    . "$INSTALL_SCRIPT"
    remote_load_configuration
    INSTALL_DIR="$SELECTED_INSTALL"
    REPO_URL="$ORIGIN"
    git -C "$INSTALL_DIR" reset --hard origin/main >/dev/null
    if git -C "$INSTALL_DIR" describe --exact-match --tags HEAD >/dev/null 2>&1; then
        fail "legacy installer fixture is not on untagged main"
    fi
    pruned_state="$TEST_DIR/retracted-release-state"
    remote_replace_process() {
        printf '%s\n%s\n' \
            "$FUTRX_CHECKOUT_REF" \
            "$(git -C "$INSTALL_DIR" tag --list 0.21.1)" > "$pruned_state"
    }

    remote_select_checkout remote.example.com --skip-dns-check
)
mapfile -t pruned_state < "$TEST_DIR/retracted-release-state"
[ "${pruned_state[0]}" = "0.21.0" ] ||
    fail "production checkout selected a retracted higher release tag"
[ -z "${pruned_state[1]}" ] ||
    fail "production checkout did not prune a retracted release tag"

# Old entry points marked themselves re-executed/selected without carrying an
# immutable ref. The new scripts must treat that as a legacy handoff and select
# the latest release instead of building whatever origin/main contained.
(
    unset __FUTRX_DEFAULTS_SOURCED FUTRX_UPDATE_SELECTED_REF FUTRX_CHECKOUT_REF
    export FUTRX_UPDATE_REEXECED=1
    # shellcheck source=../update.sh
    . "$UPDATE_SCRIPT"
    remote_load_configuration
    INSTALL_DIR="$SELECTED_INSTALL"
    remote_parse_update_arguments --skip-workspaces
    legacy_update_state="$TEST_DIR/legacy-update-handoff-state"
    exec() {
        printf '%s\n' "$FUTRX_UPDATE_SELECTED_REF" > "$legacy_update_state"
    }

    remote_refresh_checkout --skip-workspaces
)
[ "$(<"$TEST_DIR/legacy-update-handoff-state")" = "0.21.0" ] ||
    fail "legacy updater handoff did not resolve an immutable release"

(
    unset __FUTRX_DEFAULTS_SOURCED FUTRX_CHECKOUT_REF
    export FUTRX_INSTALL_CHECKOUT_SELECTED=1
    # shellcheck source=../install.sh
    . "$INSTALL_SCRIPT"
    remote_load_configuration
    INSTALL_DIR="$SELECTED_INSTALL"
    REPO_URL="$ORIGIN"
    legacy_install_state="$TEST_DIR/legacy-install-handoff-state"
    remote_replace_process() {
        printf '%s\n' "$FUTRX_CHECKOUT_REF" > "$legacy_install_state"
    }

    remote_select_checkout remote.example.com --skip-dns-check
)
[ "$(<"$TEST_DIR/legacy-install-handoff-state")" = "0.21.0" ] ||
    fail "legacy installer handoff did not resolve an immutable release"

# update.sh must load shared defaults before expanding them under `set -u`.
# The second case mirrors the in-app launcher, which supplies only the primary
# install directory.
if ! env -u __FUTRX_DEFAULTS_SOURCED \
    -u FUTRX_DEFAULT_INSTALL_DIR -u FUTRX_DEFAULT_LEGACY_INSTALL_DIR \
    -u FUTRX_INSTALL_DIR -u FUTRX_LEGACY_INSTALL_DIR \
    bash "$UPDATE_SCRIPT" --help >"$TEST_DIR/update-default.out" 2>"$TEST_DIR/update-default.err"; then
    fail "update.sh could not initialize from shared defaults: $(<"$TEST_DIR/update-default.err")"
fi
grep -q 'full infrastructure update' "$TEST_DIR/update-default.out" || \
    fail "update.sh default initialization did not reach argument parsing"

if ! env -u __FUTRX_DEFAULTS_SOURCED \
    -u FUTRX_DEFAULT_INSTALL_DIR -u FUTRX_DEFAULT_LEGACY_INSTALL_DIR \
    -u FUTRX_LEGACY_INSTALL_DIR FUTRX_INSTALL_DIR="$TEST_DIR/install" \
    bash "$UPDATE_SCRIPT" --help >"$TEST_DIR/update-launcher.out" 2>"$TEST_DIR/update-launcher.err"; then
    fail "update.sh rejected the in-app launch environment: $(<"$TEST_DIR/update-launcher.err")"
fi
grep -q 'full infrastructure update' "$TEST_DIR/update-launcher.out" || \
    fail "update.sh in-app initialization did not reach argument parsing"

# update.sh also re-executes itself after selecting the requested release.
# Preserve the target ref and flags so the selected checkout does not fall
# lose the selected release on its second pass.
(
    unset __FUTRX_DEFAULTS_SOURCED FUTRX_UPDATE_REEXECED FUTRX_UPDATE_SELECTED_REF
    # shellcheck source=../update.sh
    . "$UPDATE_SCRIPT"
    remote_load_configuration
    INSTALL_DIR="$TEST_DIR/update-reexec-install"
    mkdir -p "$INSTALL_DIR/.git"
    target_ref="0.16.1"
    original_args=(remote.example.com "--ref=$target_ref" --skip-workspaces)
    remote_parse_update_arguments "${original_args[@]}"
    exec_args="$TEST_DIR/update-reexec-args"
    expected_args="$TEST_DIR/update-reexec-expected"

    log() { :; }
    git() {
        if [[ " $* " == *" rev-parse "* ]]; then
            printf '%s\n' "0123456789abcdef0123456789abcdef01234567"
        fi
        return 0
    }
    exec() {
        printf '%s\0' "$@" >"$exec_args"
    }

    remote_refresh_checkout "${original_args[@]}" >/dev/null
    printf '%s\0' bash "$INSTALL_DIR/infra/update.sh" "${original_args[@]}" >"$expected_args"
    cmp "$expected_args" "$exec_args" || fail "updater checkout re-exec dropped arguments"
)

# install.sh must derive and export argument-dependent values after parsing.
# Caddy consumes HOSTNAME and HOSTNAME_RE through envsubst; checkout selection
# consumes FUTRX_CHECKOUT_REF during the next phase.
(
    unset __FUTRX_DEFAULTS_SOURCED HOSTNAME HOSTNAME_RE FUTRX_CHECKOUT_REF
    unset GITHUB_TOKEN GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET
    # shellcheck source=../install.sh
    . "$INSTALL_SCRIPT"
    remote_load_configuration
    target_ref="0123456789abcdef0123456789abcdef01234567"
    remote_parse_install_arguments remote.example.com \
        "--ref=$target_ref" \
        --skip-dns-check \
        --google-client-id=test-client \
        --google-client-secret=test-secret \
        --github-token=test-token
    remote_finalize_install_configuration

    [ "$HOSTNAME" = "remote.example.com" ] || fail "installer lost parsed hostname"
    [ "$HOSTNAME_RE" = 'remote\.example\.com' ] || \
        fail "installer derived incorrect hostname regex: $HOSTNAME_RE"
    [ "$FUTRX_CHECKOUT_REF" = "$target_ref" ] || \
        fail "installer did not select parsed immutable ref"
    [ "$(printenv HOSTNAME)" = "$HOSTNAME" ] || fail "HOSTNAME is not exported"
    [ "$(printenv HOSTNAME_RE)" = "$HOSTNAME_RE" ] || fail "HOSTNAME_RE is not exported"
    [ "$(printenv FUTRX_CHECKOUT_REF)" = "$target_ref" ] || fail "checkout ref is not exported"
    [ "$GOOGLE_CLIENT_ID" = "test-client" ] || fail "installer lost Google client ID"
    [ "$GOOGLE_CLIENT_SECRET" = "test-secret" ] || fail "installer lost Google client secret"
    [ "$GITHUB_TOKEN" = "test-token" ] || fail "installer lost GitHub token"
    if printenv GOOGLE_CLIENT_SECRET >/dev/null || printenv GITHUB_TOKEN >/dev/null; then
        fail "installer exported argument-provided secrets to child processes"
    fi
)

# An updater-provided checkout ref must survive when install.sh receives no
# direct --ref argument.
(
    unset __FUTRX_DEFAULTS_SOURCED
    export FUTRX_CHECKOUT_REF="0.16.1"
    # shellcheck source=../install.sh
    . "$INSTALL_SCRIPT"
    remote_load_configuration
    remote_parse_install_arguments remote.example.com --skip-dns-check
    remote_finalize_install_configuration
    [ "$FUTRX_CHECKOUT_REF" = "0.16.1" ] || fail "installer replaced inherited checkout ref"
)

# Pin the production phase order as well as the individual function behavior.
main_body="$(sed -n '/^main() {$/,/^}$/p' "$INSTALL_SCRIPT")"
parse_line="$(awk '/remote_parse_install_arguments/{ print NR; exit }' <<<"$main_body")"
finalize_line="$(awk '/remote_finalize_install_configuration/{ print NR; exit }' <<<"$main_body")"
root_line="$(awk '/require_root/{ print NR; exit }' <<<"$main_body")"
[ -n "$parse_line" ] && [ -n "$finalize_line" ] && [ -n "$root_line" ] || \
    fail "installer main is missing parse/finalize/root phases"
if [ "$parse_line" -ge "$finalize_line" ] || [ "$finalize_line" -ge "$root_line" ]; then
    fail "installer must parse arguments, finalize derived configuration, then require root"
fi

# The checkout phase re-executes install.sh. Preserve non-sensitive arguments
# in argv, while transferring credentials through an inherited descriptor so
# they are not exposed in the selected root process's command line.
(
    unset __FUTRX_DEFAULTS_SOURCED FUTRX_INSTALL_CHECKOUT_SELECTED
    # shellcheck source=../install.sh
    . "$INSTALL_SCRIPT"
    remote_load_configuration
    INSTALL_DIR="$TEST_DIR/reexec-install"
    mkdir -p "$INSTALL_DIR/.git"
    target_ref="0123456789abcdef0123456789abcdef01234567"
    export FUTRX_CHECKOUT_REF="$target_ref"
    exec_args="$TEST_DIR/reexec-args"
    expected_args="$TEST_DIR/reexec-expected"
    received_github_token="$TEST_DIR/reexec-github-token"
    received_google_secret="$TEST_DIR/reexec-google-secret"

    log() { :; }
    err() { :; }
    git() {
        if [[ " $* " == *" rev-parse "* ]]; then
            printf '%s\n' "$target_ref"
        fi
        return 0
    }
    remote_replace_process() {
        printf '%s\0' "$@" >"$exec_args"
        secrets_fd="${FUTRX_INSTALL_REEXEC_SECRETS_FD:-}"
        [ -n "$secrets_fd" ] || fail "checkout re-exec omitted credential descriptor"
        IFS= read -r -d '' descriptor_github_token <&"$secrets_fd" ||
            fail "checkout re-exec descriptor omitted GitHub token"
        IFS= read -r -d '' descriptor_google_secret <&"$secrets_fd" ||
            fail "checkout re-exec descriptor omitted Google client secret"
        printf '%s' "$descriptor_github_token" >"$received_github_token"
        printf '%s' "$descriptor_google_secret" >"$received_google_secret"
    }

    original_args=(
        remote.example.com
        --skip-dns-check
        "--ref=$target_ref"
        --google-client-id=test-client
        --google-client-secret=test-secret
        --github-token=test-token
    )
    remote_select_checkout "${original_args[@]}"
    printf '%s\0' bash "$INSTALL_DIR/infra/install.sh" \
        remote.example.com \
        --skip-dns-check \
        "--ref=$target_ref" \
        --google-client-id=test-client >"$expected_args"
    cmp "$expected_args" "$exec_args" || fail "checkout re-exec forwarded incorrect arguments"
    [ "$(<"$received_github_token")" = "test-token" ] ||
        fail "checkout re-exec descriptor lost GitHub token"
    [ "$(<"$received_google_secret")" = "test-secret" ] ||
        fail "checkout re-exec descriptor lost Google client secret"
    if grep -aFq 'test-token' "$exec_args" || grep -aFq 'test-secret' "$exec_args"; then
        fail "checkout re-exec exposed credentials in argv"
    fi
)

# The selected installer consumes the descriptor before loading configuration,
# restores both values as unexported shell state, and closes the descriptor.
(
    unset __FUTRX_DEFAULTS_SOURCED GITHUB_TOKEN GOOGLE_CLIENT_SECRET
    # shellcheck source=../install.sh
    . "$INSTALL_SCRIPT"
    exec {secrets_fd}< <(printf '%s\0%s\0' test-token test-secret)
    export FUTRX_INSTALL_REEXEC_SECRETS_FD="$secrets_fd"
    remote_receive_reexec_secrets
    remote_load_configuration

    [ "$GITHUB_TOKEN" = "test-token" ] || fail "selected installer lost GitHub token"
    [ "$GOOGLE_CLIENT_SECRET" = "test-secret" ] ||
        fail "selected installer lost Google client secret"
    if printenv GITHUB_TOKEN >/dev/null || printenv GOOGLE_CLIENT_SECRET >/dev/null; then
        fail "selected installer exported received credentials"
    fi
)

echo "Production release entrypoint tests passed"
