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

# release_highest_tag
# Reads tag names from standard input and prints the highest complete numeric
# release. The optional "v" prefix is removed only for ordering, so mixed
# prefixed/unprefixed tags compare by their numeric version rather than by raw
# refname. For an equal version, the unprefixed spelling wins deterministically.
release_highest_tag() {
    local ranked tag
    ranked="$(
        while IFS= read -r tag; do
            if release_version_train "$tag" >/dev/null; then
                printf '%s\t%s\n' "${tag#v}" "$tag"
            fi
        done | LC_ALL=C sort -t $'\t' -k1,1Vr -k2,2 | sed -n '1p'
    )"
    [ -n "$ranked" ] || return 1
    printf '%s\n' "${ranked#*$'\t'}"
}

# release_latest_tag REPOSITORY
# Prints the highest complete numeric release tag available in a checkout.
# Non-release tags and legacy two-component tags are ignored.
release_latest_tag() {
    local repository="$1"
    git -C "$repository" tag --list '[0-9]*' 'v[0-9]*' |
        release_highest_tag
}

# release_candidate_baseline REPOSITORY COMMIT
# Prints the highest release incorporated into a candidate. A normal release
# is an ancestor of the candidate. In the main <- qa release flow, the tagged
# merge commit is not merged back into qa, so its merged QA parent is the
# candidate's ancestor instead.
release_candidate_baseline() {
    local repository="$1" commit="$2"
    local tag tag_commit parent
    local -a lineage
    git -C "$repository" tag --list '[0-9]*' 'v[0-9]*' |
        while IFS= read -r tag; do
            tag_commit="$(git -C "$repository" rev-parse "${tag}^{commit}")" || continue
            if git -C "$repository" merge-base --is-ancestor "$tag_commit" "$commit"; then
                printf '%s\n' "$tag"
                continue
            fi

            read -r -a lineage <<< "$(git -C "$repository" rev-list --parents -n 1 "$tag_commit")"
            [ "${#lineage[@]}" -ge 3 ] || continue
            for parent in "${lineage[@]:1}"; do
                if git -C "$repository" merge-base --is-ancestor "$parent" "$commit"; then
                    printf '%s\n' "$tag"
                    break
                fi
            done
        done |
        release_highest_tag
}

# release_build_version REPOSITORY SELECTED_REF
# Prints the version that should be embedded in a backend built from HEAD.
# A selected release tag is preserved exactly, even if another release tag
# points at the same commit. An intentional 40-character candidate SHA is
# represented by a qa-prefixed release baseline and short SHA. Without an
# explicit ref, an exact release tag is preferred and an untagged developer
# checkout is labelled "dev". A candidate without a reachable release keeps
# the legacy commit-only QA shape.
release_build_version() {
    local repository="$1"
    local selected_ref="${2:-}"
    local head_commit selected_commit tag

    head_commit="$(git -C "$repository" rev-parse --verify 'HEAD^{commit}')" || return 1

    if release_version_train "$selected_ref" >/dev/null 2>&1; then
        selected_commit="$(
            git -C "$repository" rev-parse --verify --quiet \
                "refs/tags/${selected_ref}^{commit}"
        )" || return 1
        [ "$selected_commit" = "$head_commit" ] || return 1
        printf '%s\n' "$selected_ref"
        return 0
    fi

    if printf '%s' "$selected_ref" | grep -qE '^[0-9a-fA-F]{40}$'; then
        selected_commit="$(
            git -C "$repository" rev-parse --verify --quiet \
                "${selected_ref}^{commit}"
        )" || return 1
        [ "$selected_commit" = "$head_commit" ] || return 1
        local baseline short_commit
        short_commit="$(git -C "$repository" rev-parse --short=12 "$head_commit")"
        baseline="$(release_candidate_baseline "$repository" "$head_commit" || true)"
        if [ -n "$baseline" ]; then
            printf 'qa-%s-%s\n' "$baseline" "$short_commit"
        else
            printf 'qa-%s\n' "$short_commit"
        fi
        return 0
    fi

    if tag="$(
        git -C "$repository" tag --points-at "$head_commit" \
            --list '[0-9]*' 'v[0-9]*' | release_highest_tag
    )"; then
        printf '%s\n' "$tag"
        return 0
    fi

    printf 'dev\n'
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
