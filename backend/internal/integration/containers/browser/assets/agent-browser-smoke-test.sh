# Sanity check the GUI toolchain and select the browser pinned by versions.env.
which Xvfb x11vnc websockify openbox xdotool setsid
# Playwright's per-arch Chromium layout: the binary lives under
# chrome-linux64/ on x86_64 hosts and chrome-linux/ on aarch64. Probe both so
# aarch64 installs (which succeed via Playwright's arm64 build) are found
# instead of reported as "not installed".
CHROME=""
for browser_bin in /root/.cache/ms-playwright/chromium-*/chrome-linux64/chrome \
                   /root/.cache/ms-playwright/chromium-*/chrome-linux/chrome; do
    [ -x "$browser_bin" ] || continue
    if "$browser_bin" --version 2>/dev/null | grep -Fq "$PW_CFT_VERSION"; then
        CHROME="$browser_bin"
        break
    fi
done
if [ -z "$CHROME" ]; then
    echo "pinned Chrome for Testing $PW_CFT_VERSION was not installed" >&2
    exit 1
fi

# A present executable is insufficient: the 0.3.0 image shipped a browser that
# opened CDP and then immediately died with SIGTRAP. Keep the exact production
# launch flags alive long enough to prove the browser core is usable before an
# image can be published.
SMOKE_DIR="$(mktemp -d /tmp/remote-browser-smoke.XXXXXX)"
SMOKE_X_PID=""
SMOKE_CHROME_PID=""
stop_smoke_browser() {
    [ -n "$SMOKE_CHROME_PID" ] || return 0

    # Chrome forks helpers that can outlive its browser process briefly. Run it
    # in its own process group, stop the whole group, and wait until no helper
    # can recreate files while the temporary profile is being removed.
    kill -TERM -- "-$SMOKE_CHROME_PID" 2>/dev/null || true
    for _ in $(seq 1 50); do
        kill -0 -- "-$SMOKE_CHROME_PID" 2>/dev/null || break
        sleep 0.1
    done
    kill -KILL -- "-$SMOKE_CHROME_PID" 2>/dev/null || true
    wait "$SMOKE_CHROME_PID" 2>/dev/null || true
}

smoke_cleanup() {
    stop_smoke_browser
    if [ -n "$SMOKE_X_PID" ]; then
        kill "$SMOKE_X_PID" 2>/dev/null || true
        wait "$SMOKE_X_PID" 2>/dev/null || true
    fi
    for _ in $(seq 1 20); do
        rm -rf -- "$SMOKE_DIR" 2>/dev/null && return 0
        sleep 0.1
    done
    rm -rf -- "$SMOKE_DIR"
}
trap smoke_cleanup EXIT

Xvfb -displayfd 3 -screen 0 1366x768x24 -ac -nolisten tcp \
    3>"$SMOKE_DIR/display" >"$SMOKE_DIR/xvfb.log" 2>&1 &
SMOKE_X_PID=$!
for _ in $(seq 1 50); do
    [ -s "$SMOKE_DIR/display" ] && break
    kill -0 "$SMOKE_X_PID" 2>/dev/null || break
    sleep 0.1
done
if [ ! -s "$SMOKE_DIR/display" ]; then
    echo "browser smoke test could not start Xvfb" >&2
    tail -40 "$SMOKE_DIR/xvfb.log" >&2 || true
    exit 1
fi

DISPLAY=":$(cat "$SMOKE_DIR/display")" setsid "$CHROME" \
    --user-data-dir="$SMOKE_DIR/profile" \
    --no-sandbox --no-first-run --no-default-browser-check \
    --disable-dev-shm-usage \
    --use-gl=angle --use-angle=swiftshader-webgl \
    --renderer-process-limit=4 \
    --disable-background-networking \
    --disable-features=Translate,MediaRouter,OptimizationHints \
    --metrics-recording-only --mute-audio \
    --remote-debugging-port=19222 \
    --window-position=0,0 --window-size=1366,768 \
    about:blank >"$SMOKE_DIR/chrome.log" 2>&1 &
SMOKE_CHROME_PID=$!

SMOKE_READY=0
for _ in $(seq 1 30); do
    if curl -sf --max-time 1 http://127.0.0.1:19222/json/version >/dev/null 2>&1; then
        SMOKE_READY=1
        break
    fi
    if ! kill -0 "$SMOKE_CHROME_PID" 2>/dev/null; then
        break
    fi
    sleep 1
done
if [ "$SMOKE_READY" -ne 1 ]; then
    echo "browser smoke test failed: Chrome did not keep CDP port 19222 ready" >&2
    tail -80 "$SMOKE_DIR/chrome.log" >&2 || true
    exit 1
fi

# Catch the observed starts-then-SIGTRAP failure rather than accepting a
# momentary successful curl.
sleep 3
if ! kill -0 "$SMOKE_CHROME_PID" 2>/dev/null || \
   ! curl -sf --max-time 1 http://127.0.0.1:19222/json/version >/dev/null 2>&1; then
    echo "browser smoke test failed: Chrome exited after initially opening CDP" >&2
    tail -80 "$SMOKE_DIR/chrome.log" >&2 || true
    exit 1
fi

smoke_cleanup
trap - EXIT
echo "Agent Browser smoke test passed with Chrome for Testing $PW_CFT_VERSION"
