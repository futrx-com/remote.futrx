#!/usr/bin/env bash
# Conversation migration is an explicit offline phase, outside startup's
# HTTP health deadline. The updater's existing run freeze remains in effect.

remote_storage_version() {
    "$1" storage-version 2>/dev/null
}

remote_check_storage_compatibility() {
    local binary="$1" data_dir="$2"
    if [ -e "$data_dir/conversations.sqlite" ] && \
       [ "$(remote_storage_version "$binary" || true)" != "conversations-sqlite-v1" ]; then
        echo "Refusing a JSONL-only application over an existing conversation database." >&2
        return 1
    fi
}

remote_migrate_conversations() {
    local binary="$1" data_dir="$2" service_name="$3"
    remote_check_storage_compatibility "$binary" "$data_dir" || return 1
    if [ "$(remote_storage_version "$binary" || true)" != "conversations-sqlite-v1" ]; then
        return 0
    fi
    echo "==> Migrating conversation storage (backend stopped; resumable)"
    systemctl stop "$service_name" || return 1
    DATA_DIR="$data_dir" "$binary" migrate-chats
}

remote_storage_rollback_safe() {
    local candidate="$1" previous="$2" data_dir="$3" status
    [ -e "$data_dir/conversations.sqlite" ] || return 0
    if [ "$(remote_storage_version "$previous" || true)" = "conversations-sqlite-v1" ]; then
        return 0
    fi
    status="$(DATA_DIR="$data_dir" "$candidate" storage-status 2>/dev/null)" || {
        echo "Storage status is uncertain; automatic rollback is blocked." >&2
        return 1
    }
    case "$status" in
        legacy|importing) return 0 ;;
    esac
    echo "SQLite cutover is complete; restoring a JSONL-only binary would hide newer conversations. Service remains stopped; recover with a SQLite-compatible release." >&2
    return 1
}
