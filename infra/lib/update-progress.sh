#!/usr/bin/env bash
# Atomically publish non-secret updater progress for the admin status API.

update_progress_json_escape() {
    local value="$1"
    value="${value//\\/\\\\}"
    value="${value//\"/\\\"}"
    value="${value//$'\n'/\\n}"
    printf '%s' "$value"
}

write_update_progress() {
    local phase="$1" message="$2" progress_path progress_dir temporary
    progress_path="${FUTRX_UPDATE_PROGRESS_PATH:-}"
    [ -n "$progress_path" ] || return 0
    progress_dir="$(dirname "$progress_path")"
    mkdir -p "$progress_dir"
    chmod 700 "$progress_dir"
    temporary="${progress_path}.tmp.$$"
    printf '{"phase":"%s","message":"%s","updatedAt":%d}\n' \
        "$(update_progress_json_escape "$phase")" \
        "$(update_progress_json_escape "$message")" \
        "$(date +%s)" > "$temporary"
    chmod 600 "$temporary"
    mv "$temporary" "$progress_path"
}
