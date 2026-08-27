#!/usr/bin/env bash
# Helpers for choosing a release deployment path from numeric version tags.
# This file is sourced by deploy/update scripts; do not enable shell options.

# release_version_train VERSION
# Prints MAJOR.MINOR for a version with at least major, minor, and patch
# components. A leading "v" and legacy fourth components are accepted.
release_version_train() {
    local version="${1#v}"
    if ! printf '%s' "$version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(\.[0-9]+)*$'; then
        return 1
    fi
    local major minor remainder
    IFS=. read -r major minor remainder <<EOF
$version
EOF
    printf '%s.%s\n' "$major" "$minor"
}

# release_update_kind CURRENT TARGET
# Prints "application" only when both versions are complete numeric releases
# in the same major/minor line. Unknown and legacy two-component versions take
# the conservative infrastructure path.
release_update_kind() {
    local current_train target_train
    current_train="$(release_version_train "$1")" || {
        printf 'infrastructure\n'
        return 0
    }
    target_train="$(release_version_train "$2")" || {
        printf 'infrastructure\n'
        return 0
    }
    if [ "$current_train" = "$target_train" ]; then
        printf 'application\n'
    else
        printf 'infrastructure\n'
    fi
}
