#!/usr/bin/env bash
# Catalog-driven host agent convergence, frontend/backend build, and Google
# OAuth config seed. Checkout selection is completed by 00-checkout.sh.
#
# The selected checkout is immutable for this run, so host CLIs and the
# embedded application are built from the same module catalog.
#
# Expects from caller:
#   - log / ok / err helpers
#   - $INSTALL_DIR, $HOSTNAME
#   - $GOOGLE_CLIENT_ID, $GOOGLE_CLIENT_SECRET (optional)
#
# Sets:
#   - $AUTH_NOTE — string for the install summary
set -euo pipefail

cd "$INSTALL_DIR"

# ───────────────── agent CLIs (host-side execution/auth) ─────────────────
# This must run after checkout selection: the validated module catalog in the
# selected commit is the source of truth for host-scoped CLI policy.
log "Converging configured host agent CLIs"
(
    cd backend
    go run ./cmd/install-host-agents --prefix "$HOST_CLI_PREFIX"
)
ok "configured host agent CLIs match the selected module catalog"

# ───────────────── build ─────────────────
log "Building frontend (frontend/ → backend/public/)"
(
    cd frontend
    npm install --silent --no-audit --no-fund 2>&1 | tail -3
    npm run build 2>&1 | tail -5
)

log "Building backend (Go → backend/remote)"
(
    cd backend
    APP_VERSION="$(git -C .. describe --tags --always --dirty 2>/dev/null || echo dev)"
    go build -trimpath \
        -ldflags="-s -w -X github.com/futrx-com/remote.futrx.com/internal/version.Version=${APP_VERSION}" \
        -o remote ./cmd/remote
)
ok "$(ls -lh backend/remote | awk '{print $5}') binary"

# ───────────────── data dir + OAuth seed ─────────────────
mkdir -p "$INSTALL_DIR/data"
chmod 0700 "$INSTALL_DIR/data"

if [ -n "${GOOGLE_CLIENT_ID:-}" ] && [ -n "${GOOGLE_CLIENT_SECRET:-}" ]; then
    log "Writing Google OAuth config (data/oauth.json)"
    SECRET_ESC=$(printf '%s' "$GOOGLE_CLIENT_SECRET" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))' 2>/dev/null \
                 || printf '%s' "\"$GOOGLE_CLIENT_SECRET\"")
    cat > "$INSTALL_DIR/data/oauth.json" <<EOF
{
  "googleClientId": "$GOOGLE_CLIENT_ID",
  "googleClientSecret": $SECRET_ESC
}
EOF
    chmod 0600 "$INSTALL_DIR/data/oauth.json"
    ok "Google sign-in enabled for invited users"
    AUTH_NOTE="Local admin authentication enabled. Google sign-in is ready for invited users."
elif [ -s "$INSTALL_DIR/data/oauth.json" ]; then
    ok "Google sign-in enabled for invited users (using the existing configuration)"
    AUTH_NOTE="Local admin authentication enabled. Existing Google user sign-in settings were preserved."
else
    AUTH_NOTE="Local admin authentication enabled. Create the admin password on first visit."
fi
export AUTH_NOTE
