#!/usr/bin/env bash
# Resolve agent CLI releases and persist the exact versions used by an install.
# This file is sourced by the host dependency step and by its focused tests.

agent_cli_semver() {
    "$1" --version 2>&1 \
        | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?' \
        | head -1 || true
}

require_agent_cli_semver() {
    local label="$1" version="$2"
    if ! printf '%s\n' "$version" \
        | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
        echo "could not resolve a valid $label version (got: ${version:-empty})" >&2
        return 1
    fi
}

latest_npm_package_version() {
    local package="$1" version
    version="$(npm view "$package" version 2>/dev/null | tail -1 | tr -d '[:space:]')"
    require_agent_cli_semver "$package" "$version" || return 1
    printf '%s\n' "$version"
}

latest_antigravity_cli_version() {
    local machine platform manifest version
    machine="$(uname -m)"
    case "$machine" in
        x86_64|amd64) platform="linux_amd64" ;;
        aarch64|arm64) platform="linux_arm64" ;;
        *)
            echo "unsupported architecture for Antigravity: $machine" >&2
            return 1
            ;;
    esac

    manifest="$(curl -fsSL \
        "https://antigravity-cli-auto-updater-974169037036.us-central1.run.app/manifests/${platform}.json")"
    version="$(printf '%s' "$manifest" | jq -er '.version')"
    require_agent_cli_semver "Antigravity CLI" "$version" || return 1
    printf '%s\n' "$version"
}

# write_agent_cli_versions MANIFEST CLAUDE CODEX KIMI ANTIGRAVITY
#
# Rewrites all four values in one atomic replacement. realpath deliberately
# resolves infra/versions.env before replacement so its symlink is preserved.
write_agent_cli_versions() {
    local manifest="$1" claude="$2" codex="$3" kimi="$4" antigravity="$5"
    local canonical_manifest tmp

    require_agent_cli_semver "Claude Code" "$claude" || return 1
    require_agent_cli_semver "Codex CLI" "$codex" || return 1
    require_agent_cli_semver "Kimi Code" "$kimi" || return 1
    require_agent_cli_semver "Antigravity CLI" "$antigravity" || return 1

    if [ ! -f "$manifest" ]; then
        echo "version manifest not found: $manifest" >&2
        return 1
    fi
    canonical_manifest="$(realpath "$manifest")"
    tmp="$(mktemp "${canonical_manifest}.tmp.XXXXXX")"

    if ! awk \
        -v claude="$claude" \
        -v codex="$codex" \
        -v kimi="$kimi" \
        -v antigravity="$antigravity" '
        BEGIN {
            replacement["CLAUDE_CODE_VERSION"] = claude
            replacement["CODEX_CLI_VERSION"] = codex
            replacement["KIMI_CODE_VERSION"] = kimi
            replacement["ANTIGRAVITY_CLI_VERSION"] = antigravity
        }
        {
            line = $0
            split(line, fields, "=")
            key = fields[1]
            if (key in replacement) {
                print key "=" replacement[key]
                seen[key]++
            } else {
                print line
            }
        }
        END {
            for (key in replacement) {
                if (seen[key] != 1) {
                    print "version manifest must contain exactly one " key > "/dev/stderr"
                    exit 1
                }
            }
        }
    ' "$canonical_manifest" > "$tmp"; then
        command rm -f -- "$tmp"
        return 1
    fi

    chmod 0644 "$tmp"
    mv -f -- "$tmp" "$canonical_manifest"
}
