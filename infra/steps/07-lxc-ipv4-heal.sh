#!/usr/bin/env bash
# Self-heal for containers that silently lose their IPv4 lease.
#
# Under host load, in-container systemd-networkd can hit rtnl timeouts
# ("Could not set NDisc address/route: Connection timed out"), mark eth0
# failed, flush the DHCP address + default route, and never retry. The
# container stays RUNNING with IPv6 link-local only, so agent chats die
# with "Network unreachable (os error 101)". Observed 2026-07-13/14 on
# three containers within a day.
#
# A minutely timer reconfigures eth0 on any RUNNING container with no
# IPv4. Reconfigure is a no-op-grade action on healthy containers; a
# 90s boot grace avoids racing a legitimately in-flight first DHCP.
# Manual per-project trigger also exists in the UI (workspace info →
# Network → Repair network).
#
# Expects from caller: log / ok helpers.
set -euo pipefail

step_07_lxc_ipv4_heal() {
log "Installing lxc-ipv4-heal (container IPv4 self-heal)"

cat > /usr/local/sbin/lxc-ipv4-heal << "SCRIPT"
#!/bin/bash
# Reconfigure eth0 on RUNNING LXD containers that have no IPv4 address.
# Installed by infra/steps/07-lxc-ipv4-heal.sh; runs from lxc-ipv4-heal.timer.
set -u
while IFS=, read -r name state ipv4; do
  [ "$state" = "RUNNING" ] || continue
  [ -z "$ipv4" ] || continue
  pid=$(lxc info "$name" 2>/dev/null | awk "/^PID:/ {print \$2}")
  if [ -n "${pid:-}" ] && [ -d "/proc/$pid" ]; then
    started=$(stat -c %Y "/proc/$pid" 2>/dev/null || echo 0)
    now=$(date +%s)
    [ $((now - started)) -lt 90 ] && continue
  fi
  logger -t lxc-ipv4-heal "container $name RUNNING with no IPv4 — reconfiguring eth0"
  lxc exec "$name" -- networkctl reconfigure eth0
done < <(lxc list -c ns4 --format csv | sed "s/ (eth0)//")
SCRIPT
chmod +x /usr/local/sbin/lxc-ipv4-heal

cat > /etc/systemd/system/lxc-ipv4-heal.service << "UNIT"
[Unit]
Description=Reconfigure eth0 on LXD containers that lost their IPv4 lease

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/lxc-ipv4-heal
UNIT

cat > /etc/systemd/system/lxc-ipv4-heal.timer << "UNIT"
[Unit]
Description=Sweep LXD containers for lost IPv4 every minute

[Timer]
OnBootSec=2min
OnUnitActiveSec=1min
AccuracySec=15s

[Install]
WantedBy=timers.target
UNIT

systemctl daemon-reload
systemctl enable --now lxc-ipv4-heal.timer

ok "lxc-ipv4-heal timer active"
}
