#!/bin/sh
# Agent Browser launcher - runs inside the project container.
#
# Core: Xvfb + openbox + headed Chromium + loopback CDP.
# View: x11vnc + websockify/noVNC for the human Browser pane.
#
# Usage:
#   gui-up.sh start|stop|restart|status
#   gui-up.sh start-core|stop-core|restart-core
#   gui-up.sh start-view|stop-view|restart-view
set -u

GUI_DIR=/workspace/.browser-gui
PROFILE="$GUI_DIR/profile"
LOG="$GUI_DIR/gui.log"
CORE_STARTED_AT="$GUI_DIR/core.started_at"
DISPLAY_NUM=99
VNC_PORT=6080
RFB_PORT=5900
CDP_PORT=9222
SCREEN=1366x768x24
EXPECTED_CHROME_VERSION=__PW_CFT_VERSION__
export DISPLAY=":$DISPLAY_NUM"

# Prefer Playwright's Chromium (Chrome for Testing) — the baked-in default,
# and its path matches no Ubuntu AppArmor browser profile so it networks in
# any container. The google-chrome fallback needs the /etc/apparmor.d/local/
# chrome "network," rule (baked by AgentBrowserInstallScript since 2026-07-08;
# Ubuntu's stock chrome profile otherwise denies all its sockets inside
# nested LXD AppArmor namespaces — CreatePlatformSocket EPERM).
# Playwright's per-arch Chromium layout: the binary lives under
# chrome-linux64/ on x86_64 hosts and chrome-linux/ on aarch64. Probe both so
# aarch64 installs (which succeed via Playwright's arm64 build) are found.
CHROME=""
for browser_bin in /root/.cache/ms-playwright/chromium-*/chrome-linux64/chrome \
                   /root/.cache/ms-playwright/chromium-*/chrome-linux/chrome; do
  [ -x "$browser_bin" ] || continue
  if "$browser_bin" --version 2>/dev/null | grep -Fq "$EXPECTED_CHROME_VERSION"; then
    CHROME="$browser_bin"
    break
  fi
done
if [ -z "$CHROME" ]; then
  echo "[gui-up] pinned Chrome for Testing $EXPECTED_CHROME_VERSION is not installed" >&2
  exit 1
fi

log() { echo "[gui-up] $*"; }

now_sec() { date +%s; }

core_ready() {
  curl -sf --max-time 2 "http://127.0.0.1:$CDP_PORT/json/version" >/dev/null 2>&1
}

view_ready() {
  curl -sf --max-time 2 "http://127.0.0.1:$VNC_PORT/vnc.html" >/dev/null 2>&1
}

viewer_count() {
  if ! command -v ss >/dev/null 2>&1; then
    echo 0
    return
  fi
  ss -H -tn state established "( sport = :$RFB_PORT )" 2>/dev/null | wc -l | tr -d ' '
}

uptime_sec() {
  if [ ! -r "$CORE_STARTED_AT" ]; then
    echo 0
    return
  fi
  started="$(cat "$CORE_STARTED_AT" 2>/dev/null || echo 0)"
  case "$started" in
    ''|*[!0-9]*) echo 0 ;;
    *) echo $(( $(now_sec) - started )) ;;
  esac
}

chrome_running() {
  pgrep -f "user-data-dir=$PROFILE" >/dev/null 2>&1
}

clear_stale_profile_locks() {
  chrome_running && return 0
  # A force-deleted container cannot let Chromium clean these symlinks. Only
  # remove Chrome's known symlinks, and only while no process owns the profile;
  # the profile itself (cookies and authenticated sessions) remains untouched.
  for lock_name in SingletonLock SingletonCookie SingletonSocket; do
    [ ! -L "$PROFILE/$lock_name" ] || rm -f -- "$PROFILE/$lock_name"
  done
}

start_x_stack() {
  if ! pgrep -f "Xvfb :$DISPLAY_NUM" >/dev/null 2>&1; then
    setsid Xvfb ":$DISPLAY_NUM" -screen 0 "$SCREEN" -ac -nolisten tcp </dev/null >>"$LOG" 2>&1 &
    sleep 1
  fi

  # Window manager first, so Chrome's window gets stable input focus.
  if ! pgrep -x openbox >/dev/null 2>&1; then
    setsid openbox </dev/null >>"$LOG" 2>&1 &
    sleep 1
  fi
}

launch_chrome_direct() {
  setsid "$CHROME" \
    --user-data-dir="$PROFILE" \
    --no-sandbox --no-first-run --no-default-browser-check \
    --disable-dev-shm-usage \
    --use-gl=angle --use-angle=swiftshader-webgl \
    --renderer-process-limit=4 \
    --disable-background-networking \
    --disable-features=Translate,MediaRouter,OptimizationHints \
    --metrics-recording-only \
    --mute-audio \
    --remote-debugging-port="$CDP_PORT" \
    --window-position=0,0 --window-size=1366,768 \
    "about:blank"
}

# A transient systemd SERVICE, not a scope. A scope wrapping `setsid` is
# racy: setsid exits as soon as it forks Chrome, systemd sees the scope's
# initial process gone, tears the cgroup down, and SIGTERMs the freshly
# started Chrome - "core ready" then dead within seconds. A service
# supervises Chrome directly (no setsid needed), detaches it from this
# shell, applies the same resource caps, and appends output to $LOG.
launch_chrome_service() {
  systemctl reset-failed agent-browser.service >/dev/null 2>&1 || true
  systemd-run --quiet --collect --unit=agent-browser \
    --service-type=exec --setenv=DISPLAY="$DISPLAY" \
    -p MemoryMax=1536M -p CPUQuota=200% \
    -p "StandardOutput=append:$LOG" -p "StandardError=append:$LOG" \
    "$CHROME" \
    --user-data-dir="$PROFILE" \
    --no-sandbox --no-first-run --no-default-browser-check \
    --disable-dev-shm-usage \
    --use-gl=angle --use-angle=swiftshader-webgl \
    --renderer-process-limit=4 \
    --disable-background-networking \
    --disable-features=Translate,MediaRouter,OptimizationHints \
    --metrics-recording-only \
    --mute-audio \
    --remote-debugging-port="$CDP_PORT" \
    --window-position=0,0 --window-size=1366,768 \
    "about:blank"
}

start_core() {
  mkdir -p "$PROFILE"
  touch "$LOG"
  start_x_stack

  # Headed Chromium, persistent profile, loopback CDP port. --no-sandbox
  # because the unprivileged LXC container is itself the isolation boundary.
  if ! chrome_running; then
    clear_stale_profile_locks
    if command -v systemd-run >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
      launch_chrome_service </dev/null >>"$LOG" 2>&1 &
    else
      launch_chrome_direct </dev/null >>"$LOG" 2>&1 &
    fi
    now_sec > "$CORE_STARTED_AT"
    sleep 2
  elif [ ! -r "$CORE_STARTED_AT" ]; then
    now_sec > "$CORE_STARTED_AT"
  fi

  i=0
  while [ "$i" -lt 30 ]; do
    if core_ready; then
      log "core ready (cdp :$CDP_PORT)"
      return 0
    fi
    if ! chrome_running; then
      log "browser core exited before CDP became ready"
      if command -v journalctl >/dev/null 2>&1; then
        journalctl -u agent-browser.service -n 20 --no-pager >>"$LOG" 2>&1 || true
      fi
      stop_core
      return 1
    fi
    i=$((i + 1))
    sleep 1
  done
  log "timed out waiting for browser core to become ready"
  stop_core
  return 1
}

start_view() {
  start_core || return 1

  # x11vnc on the virtual display, loopback only. -xkb fixes key mapping and
  # -threads keeps input responsive under framebuffer load.
  if ! pgrep -x x11vnc >/dev/null 2>&1; then
    setsid x11vnc -display ":$DISPLAY_NUM" -localhost -forever -shared -nopw \
      -rfbport "$RFB_PORT" -xkb -noxrecord -threads -quiet </dev/null >>"$LOG" 2>&1 &
    sleep 1
  fi

  # noVNC web front, bound to 0.0.0.0 so the host can reach it over the LXD
  # bridge for the dev-URL proxy. It is gated by platform auth at the edge;
  # only this port is reachable from outside the container.
  if ! pgrep -f "websockify.*:$VNC_PORT" >/dev/null 2>&1; then
    setsid websockify --web=/usr/share/novnc "0.0.0.0:$VNC_PORT" "127.0.0.1:$RFB_PORT" </dev/null >>"$LOG" 2>&1 &
    sleep 1
  fi

  i=0
  while [ "$i" -lt 30 ]; do
    if view_ready; then
      log "view ready (vnc :$VNC_PORT, clients=$(viewer_count))"
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  log "timed out waiting for browser view to become ready"
  return 1
}

stop_view() {
  pkill -f "websockify.*:$VNC_PORT" 2>/dev/null
  pkill -x x11vnc 2>/dev/null
  log "view stopped"
}

stop_core() {
  stop_view
  systemctl stop agent-browser.service >/dev/null 2>&1 || true
  pkill -f "user-data-dir=$PROFILE" 2>/dev/null
  pkill -x openbox 2>/dev/null
  pkill -f "Xvfb :$DISPLAY_NUM" 2>/dev/null
  rm -f "$CORE_STARTED_AT"
  log "core stopped"
}

status() {
  if core_ready; then core=ready; else core=off; fi
  if view_ready; then view=ready; else view=off; fi
  printf '[gui-up] core=%s view=%s clients=%s uptime_sec=%s\n' "$core" "$view" "$(viewer_count)" "$(uptime_sec)"
}

case "${1:-start}" in
  start)        start_view ;;
  start-core)   start_core ;;
  start-view)   start_view ;;
  stop)         stop_core ;;
  stop-core)    stop_core ;;
  stop-view)    stop_view ;;
  restart)      stop_core; sleep 1; start_view ;;
  restart-core) stop_core; sleep 1; start_core ;;
  restart-view) stop_view; sleep 1; start_view ;;
  status)       status ;;
  *) echo "usage: $0 start|stop|restart|status|start-core|stop-core|restart-core|start-view|stop-view|restart-view" >&2; exit 2 ;;
esac
