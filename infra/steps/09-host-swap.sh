#!/usr/bin/env bash
# Add a swapfile on small hosts (< 8 GiB RAM) that have no swap at all.
#
# Project containers run Node, Chromium and agent CLIs; on a 4 GiB VPS one
# busy container can OOM the host. 4 GiB of swap with swappiness=10 is not a
# substitute for RAM but turns a hard OOM-kill into a slowdown. No-op when
# swap already exists or the host has >= 8 GiB.
#
# Expects from caller: log / ok / warn helpers.
set -euo pipefail

mem_kib=$(awk '/MemTotal/ {print $2}' /proc/meminfo)
swap_kib=$(awk '/SwapTotal/ {print $2}' /proc/meminfo)

if [ "${swap_kib:-0}" -gt 0 ]; then
    ok "swap already present ($((swap_kib / 1024)) MiB) — skipping"
elif [ "${mem_kib:-0}" -ge $((8 * 1024 * 1024)) ]; then
    ok "host has $((mem_kib / 1024 / 1024)) GiB RAM — no swapfile needed"
else
    log "Creating 4 GiB swapfile (host has $((mem_kib / 1024)) MiB RAM, no swap)"
    if [ ! -f /swapfile ]; then
        fallocate -l 4G /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=4096 status=none
        chmod 600 /swapfile
        mkswap /swapfile >/dev/null
    fi
    swapon /swapfile 2>/dev/null || true
    grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
    printf 'vm.swappiness=10\nvm.vfs_cache_pressure=50\n' > /etc/sysctl.d/90-remote-swap.conf
    sysctl -q -p /etc/sysctl.d/90-remote-swap.conf || true
    ok "swapfile active (4 GiB, swappiness=10)"
fi
