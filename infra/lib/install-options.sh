# Remember the topology choices across re-runs.
#
# infra/update.sh re-invokes infra/install.sh with only the hostname, and the
# documented recovery for almost everything is "re-run the installer". Neither
# carries the flags of the original install, so without this an operator who
# ran with --no-plesk-integration or --caddy-port=8081 would silently get a
# different topology on their next update — Caddy moving back behind Plesk, or
# nginx pointed at a port nothing is listening on.
#
# Only choices that cannot be re-derived from the host are stored. Everything
# else (whether Plesk is installed, which certificate covers the wildcards,
# what Plesk's nginx binds) is detected fresh every run, because it can change
# underneath us.
#
# Deliberately outside INSTALL_DIR: steps/02-app.sh hard-resets that checkout
# to origin/main on every run.

FUTRX_OPTIONS_FILE="${FUTRX_OPTIONS_FILE:-/etc/remote.futrx/install-options.env}"

# load_install_options
# Sets FUTRX_SAVED_* from the file, if it exists. Callers apply them only
# where the operator did not pass the flag explicitly, so an explicit flag
# always wins over a remembered one.
load_install_options() {
    FUTRX_SAVED_NO_PLESK_INTEGRATION=""
    FUTRX_SAVED_CADDY_HTTP_PORT=""
    FUTRX_SAVED_FORCE_SSH_HARDENING=""
    [ -r "$FUTRX_OPTIONS_FILE" ] || return 0

    local key value
    while IFS='=' read -r key value; do
        case "$key" in
            NO_PLESK_INTEGRATION) FUTRX_SAVED_NO_PLESK_INTEGRATION="$value" ;;
            CADDY_HTTP_PORT)      FUTRX_SAVED_CADDY_HTTP_PORT="$value" ;;
            FORCE_SSH_HARDENING)  FUTRX_SAVED_FORCE_SSH_HARDENING="$value" ;;
        esac
    done < "$FUTRX_OPTIONS_FILE"
    return 0
}

# save_install_options <no_plesk_integration> <caddy_http_port> <force_ssh_hardening>
# Best-effort: a box that cannot write here still installs correctly, it just
# will not remember. Never fatal.
save_install_options() {
    local dir
    dir="$(dirname "$FUTRX_OPTIONS_FILE")"
    mkdir -p "$dir" 2>/dev/null || return 0
    {
        printf '%s\n' \
            '# Written by infra/install.sh. Read back on re-runs and by infra/update.sh,' \
            '# so an update does not silently change the topology. Safe to delete: the' \
            '# next install falls back to detection and defaults.' \
            "NO_PLESK_INTEGRATION=${1:-0}" \
            "CADDY_HTTP_PORT=${2:-8080}" \
            "FORCE_SSH_HARDENING=${3:-0}"
    } > "$FUTRX_OPTIONS_FILE" 2>/dev/null || return 0
    chmod 0644 "$FUTRX_OPTIONS_FILE" 2>/dev/null || true
    return 0
}
