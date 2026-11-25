#!/usr/bin/env bash
# Conversation migration is an explicit offline phase, outside startup's
# HTTP health deadline. The backend is stopped before conversion begins.

remote_storage_version() {
    "$1" storage-version 2>/dev/null
}

remote_check_storage_compatibility() {
    local binary="$1" data_dir="$2" schema
    [ -e "$data_dir/conversations.sqlite" ] || return 0
    if [ "$(remote_storage_version "$binary" || true)" != "conversations-sqlite-v2" ]; then
        echo "Refusing an application without SQLite schema-v2 compatibility over an existing conversation database." >&2
        return 1
    fi
    schema="$(DATA_DIR="$data_dir" "$binary" storage-schema 2>/dev/null)" || return 1
    case "$schema" in
        0|1|2) return 0 ;;
        *) echo "Unsupported conversation schema $schema; deployment is blocked." >&2; return 1 ;;
    esac
}

remote_migrate_conversations() {
    local binary="$1" data_dir="$2" service_name="$3"
    remote_check_storage_compatibility "$binary" "$data_dir" || return 1
    if [ "$(remote_storage_version "$binary" || true)" != "conversations-sqlite-v2" ]; then
        return 0
    fi
    echo "==> Migrating conversation storage (backend stopped; resumable)"
    systemctl stop "$service_name" || return 1
    DATA_DIR="$data_dir" "$binary" migrate-chats
}

remote_storage_rollback_safe() {
    local candidate="$1" previous="$2" data_dir="$3" status schema previous_version
    [ -e "$data_dir/conversations.sqlite" ] || return 0
    schema="$(DATA_DIR="$data_dir" "$candidate" storage-schema 2>/dev/null)" || {
        echo "Conversation schema is uncertain; automatic rollback is blocked." >&2
        return 1
    }
    previous_version="$(remote_storage_version "$previous" || true)"
    case "$previous_version:$schema" in
        conversations-sqlite-v2:0|conversations-sqlite-v2:1|conversations-sqlite-v2:2|conversations-sqlite-v1:0|conversations-sqlite-v1:1) return 0 ;;
        conversations-sqlite-*)
            echo "Conversation schema $schema is incompatible with $previous_version; automatic rollback is blocked. Recover with a compatible release and the current database." >&2
            return 1
            ;;
    esac
    case "$schema" in
        0|1|2) ;;
        *) echo "Unsupported conversation schema $schema; automatic rollback is blocked." >&2; return 1 ;;
    esac
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
