#!/usr/bin/env bash
# Install the official Debian package from a stable Caddy GitHub release.
# Its published SHA-512 checksum is checked before APT installs anything.

install_caddy_from_github() (
    set -euo pipefail
    local deb_arch release_arch tag version work_dir filename checksum
    deb_arch="$(dpkg --print-architecture)" || return 1
    case "$deb_arch" in
        amd64|arm64|riscv64|s390x) release_arch="$deb_arch" ;;
        armhf) release_arch=armv7 ;;
        armel) release_arch=armv5 ;;
        ppc64el) release_arch=ppc64le ;;
        *) echo "Unsupported Caddy package architecture: $deb_arch" >&2; return 1 ;;
    esac
    tag="$(curl -fsSL --connect-timeout 15 --max-time 60 --retry 3 \
        https://api.github.com/repos/caddyserver/caddy/releases/latest | jq -er .tag_name)" || return 1
    if ! [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo 'The official Caddy release did not identify a stable version.' >&2
        return 1
    fi
    version="${tag#v}"
    filename="caddy_${version}_linux_${release_arch}.deb"
    work_dir="$(mktemp -d)" || return 1
    trap 'rm -rf -- "$work_dir"' EXIT
    local release_url="https://github.com/caddyserver/caddy/releases/download/$tag"
    curl -fsSL --connect-timeout 15 --max-time 120 --retry 3 \
        "$release_url/caddy_${version}_checksums.txt" -o "$work_dir/checksums.txt" || return 1
    curl -fsSL --connect-timeout 15 --max-time 120 --retry 3 \
        "$release_url/$filename" -o "$work_dir/$filename" || return 1
    checksum="$(awk -v filename="$filename" '$2 == filename {print $1}' "$work_dir/checksums.txt")"
    if ! [[ "$checksum" =~ ^[[:xdigit:]]{128}$ ]]; then
        echo 'The official Caddy release checksum is missing or ambiguous.' >&2
        return 1
    fi
    if ! printf '%s  %s\n' "$checksum" "$work_dir/$filename" | sha512sum --check --status; then
        echo 'Caddy package checksum verification failed; refusing to install it.' >&2
        return 1
    fi
    if [ "$(dpkg-deb --field "$work_dir/$filename" Package)" != caddy ] || \
       [ "$(dpkg-deb --field "$work_dir/$filename" Version)" != "$version" ] || \
       [ "$(dpkg-deb --field "$work_dir/$filename" Architecture)" != "$deb_arch" ]; then
        echo 'The verified Caddy package has unexpected package metadata.' >&2
        return 1
    fi
    # The artifact is public and verified. Let APT's sandbox read the local file.
    chmod 0755 "$work_dir" || return 1
    host_apt_get install -y -qq "$work_dir/$filename" || return 1
    if [ "$(caddy version | awk '{print $1}')" != "$tag" ]; then
        echo 'The installed Caddy version does not match the verified release.' >&2
        return 1
    fi
)
