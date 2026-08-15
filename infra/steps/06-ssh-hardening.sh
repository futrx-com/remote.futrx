#!/usr/bin/env bash
# SSH hardening: public-key auth only, box-wide.
#
# Root is already `prohibit-password` out of the box, but PasswordAuthentication
# defaults to yes for every *other* (and future) account, leaving an online
# brute-force surface on the internet-facing port 22. We close that class
# entirely. No account on this box carries a usable password hash, so this
# locks in the secure default rather than changing anyone's live access.
#
# Delivered as a drop-in so we never hand-edit the distro sshd_config. sshd
# is first-match-wins and the stock config Includes sshd_config.d/*.conf at
# the very top, so the `10-` prefix guarantees our directives win over any
# later drop-in (e.g. a cloud-init file) that might re-enable passwords.
#
# The "no account carries a usable password hash" premise holds on a fresh
# server and not on a Plesk one: Plesk creates system users for subscriptions
# and its customers reach them over SSH and SFTP with passwords the panel
# issued. Disabling password auth box-wide would lock those people out of
# hosting accounts that have nothing to do with this platform, so on a Plesk
# host this is skipped unless the operator explicitly asks for it.
#
# Expects from caller: log / ok / warn / err helpers.
set -euo pipefail

if [ "${FUTRX_FRONTEND_MODE:-standalone}" = "plesk" ] && [ "${FORCE_SSH_HARDENING:-0}" -eq 0 ]; then
    warn "Plesk detected — leaving SSH password authentication enabled."
    echo "  Disabling it box-wide would lock out panel-managed SFTP and SSH users." >&2
    echo "  Port 22 keeps an online brute-force surface; Plesk's fail2ban limits it." >&2
    echo "  To harden anyway: re-run with --force-ssh-hardening" >&2
    return 0 2>/dev/null || exit 0
fi

DROPIN=/etc/ssh/sshd_config.d/10-futrx-hardening.conf

log "Hardening sshd (key-only auth)"

mkdir -p "$(dirname "$DROPIN")"
cat > "$DROPIN" <<'EOF'
# Managed by infra/steps/06-ssh-hardening.sh — do not edit by hand.
# Public-key authentication only. Disables the password and keyboard-
# interactive (PAM password) paths so no account is brute-forceable over SSH.
PasswordAuthentication no
KbdInteractiveAuthentication no
EOF
chmod 0644 "$DROPIN"

# Validate the *whole* config (main + every drop-in) before touching the
# running daemon. On any error, pull our drop-in so a re-run starts clean.
if ! sshd -t 2>/dev/null; then
    err "sshd config invalid after writing $DROPIN — reverting."
    rm -f "$DROPIN"
    sshd -t 2>&1 | tail -20 >&2 || true
    exit 1
fi

# Confirm the directive actually took effect. If the stock config didn't
# Include the drop-in dir, the file would be silently ignored — fail loudly
# instead of reporting success on a box that's still accepting passwords.
EFFECTIVE="$(sshd -T 2>/dev/null | awk '/^passwordauthentication/ {print $2}')"
if [ "$EFFECTIVE" != "no" ]; then
    err "Drop-in written but PasswordAuthentication is still '${EFFECTIVE:-unset}'."
    err "The distro sshd_config may not Include $(dirname "$DROPIN")/*.conf."
    exit 1
fi

# Reload (not restart): existing SSH sessions, including this one, survive.
# Service is `ssh` on Debian/Ubuntu; tolerate the `sshd` alias elsewhere.
if systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null; then
    :
else
    systemctl reload-or-restart ssh
fi

ok "sshd: password auth disabled (public-key only)"
