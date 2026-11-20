#!/usr/bin/env bash
set -euo pipefail

INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$INFRA_DIR/lib/chat-migration.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT
export TEST_DIR
mkdir -p "$TEST_DIR/data"
fail() { echo "FAIL: $*" >&2; exit 1; }

cat > "$TEST_DIR/current" <<'EOF'
#!/usr/bin/env bash
case "$1" in
 storage-version) echo conversations-sqlite-v2 ;;
 storage-schema) if [ -f "$DATA_DIR/schema" ]; then cat "$DATA_DIR/schema"; else echo 2; fi ;;
 storage-status) [ -f "$DATA_DIR/status" ] && cat "$DATA_DIR/status" ;;
 migrate-chats)
  [ "${MIGRATION_FAIL:-0}" != 1 ] || exit 1
  [ "$(tail -1 "$TEST_DIR/calls")" = 'stop test.service' ] || exit 9
  echo migration >> "$TEST_DIR/calls"
  touch "$DATA_DIR/conversations.sqlite"
  echo complete > "$DATA_DIR/status"
 ;;
 *) exit 1 ;;
esac
EOF
cat > "$TEST_DIR/previous" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "$TEST_DIR/current" "$TEST_DIR/previous"
cat > "$TEST_DIR/v1" <<'EOF'
#!/usr/bin/env bash
[ "$1" = storage-version ] && echo conversations-sqlite-v1
EOF
chmod +x "$TEST_DIR/v1"
systemctl() { printf '%s\n' "$*" >> "$TEST_DIR/calls"; }

# Legacy deployments without a database retain their pre-migration behavior.
remote_migrate_conversations "$TEST_DIR/previous" "$TEST_DIR/data" test.service
[ ! -e "$TEST_DIR/calls" ] || fail "legacy candidate unexpectedly stopped service"
remote_migrate_conversations "$TEST_DIR/current" "$TEST_DIR/data" test.service
printf 'stop test.service\nmigration\n' > "$TEST_DIR/expected"
cmp "$TEST_DIR/expected" "$TEST_DIR/calls" || fail "migration ran before backend stop"

if remote_check_storage_compatibility "$TEST_DIR/previous" "$TEST_DIR/data" 2>"$TEST_DIR/error"; then
 fail "old candidate accepted over SQLite"
fi
if remote_storage_rollback_safe "$TEST_DIR/current" "$TEST_DIR/previous" "$TEST_DIR/data" 2>"$TEST_DIR/error"; then
 fail "JSONL rollback accepted after completed cutover"
fi
grep -q 'cutover is complete' "$TEST_DIR/error" || fail "unsafe rollback message missing"
remote_storage_rollback_safe "$TEST_DIR/current" "$TEST_DIR/current" "$TEST_DIR/data"
if remote_storage_rollback_safe "$TEST_DIR/current" "$TEST_DIR/v1" "$TEST_DIR/data" 2>"$TEST_DIR/error"; then
 fail "schema-v1 rollback accepted after schema-v2 cutover"
fi
grep -q 'incompatible' "$TEST_DIR/error" || fail "schema rollback message missing"
echo 1 > "$TEST_DIR/data/schema"
remote_storage_rollback_safe "$TEST_DIR/current" "$TEST_DIR/v1" "$TEST_DIR/data"
echo 2 > "$TEST_DIR/data/schema"
echo importing > "$TEST_DIR/data/status"
remote_storage_rollback_safe "$TEST_DIR/current" "$TEST_DIR/previous" "$TEST_DIR/data"
rm "$TEST_DIR/data/status"
if remote_storage_rollback_safe "$TEST_DIR/current" "$TEST_DIR/previous" "$TEST_DIR/data" 2>"$TEST_DIR/error"; then
 fail "uncertain status allowed JSONL rollback"
fi
if MIGRATION_FAIL=1 remote_migrate_conversations "$TEST_DIR/current" "$TEST_DIR/data" test.service; then
 fail "migration failure swallowed"
fi
echo 3 > "$TEST_DIR/data/schema"
if remote_check_storage_compatibility "$TEST_DIR/current" "$TEST_DIR/data" 2>"$TEST_DIR/error"; then
 fail "future schema accepted by older candidate"
fi

for path in deploy-app.sh qa/deploy-app.sh qa/deploy-local.sh steps/04-backend-svc.sh; do
 bash -n "$INFRA_DIR/$path"
 grep -q 'remote_migrate_conversations' "$INFRA_DIR/$path" || fail "missing migration phase: $path"
done
echo "Conversation migration deployment tests passed"
