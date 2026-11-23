#!/usr/bin/env bash
# Refresh the shell-only source snapshot used when S3Disk is built into Remote.
set -euo pipefail

case "${1:-}" in
  ''|--check) ;;
  *) echo "usage: $0 [--check]" >&2; exit 2 ;;
esac

application_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
installer="$application_dir/infra/install.sh"
source_dir="$application_dir/backend/container"
start_marker='# BEGIN EMBEDDED S3DISK SOURCE'
end_marker='# END EMBEDDED S3DISK SOURCE'
start_line="$(grep -nF "$start_marker" "$installer" | cut -d: -f1)"
end_line="$(grep -nF "$end_marker" "$installer" | cut -d: -f1)"
if [ -z "$start_line" ] || [ -z "$end_line" ] || [ "$start_line" -ge "$end_line" ]; then
  echo 'embed-source: install.sh source markers are missing or out of order' >&2
  exit 1
fi

archive="$(mktemp)"
rendered="$(mktemp)"
trap 'rm -f -- "$archive" "$rendered"' EXIT

tar --sort=name --format=ustar --mtime='@0' --owner=0 --group=0 \
  --numeric-owner -C "$source_dir" -cf - . | gzip -n -9 >"$archive"
archive_sha256="$(sha256sum "$archive" | cut -d' ' -f1)"

head -n "$start_line" "$installer" >"$rendered"
printf "    archive_sha256='%s'\n" "$archive_sha256" >>"$rendered"
printf "    base64 --decode >\"\$archive\" <<'S3DISK_EMBEDDED_SOURCE'\n" >>"$rendered"
base64 -w 76 "$archive" >>"$rendered"
printf 'S3DISK_EMBEDDED_SOURCE\n' >>"$rendered"
tail -n "+$end_line" "$installer" >>"$rendered"

if [ "${1:-}" = --check ]; then
  cmp -s "$rendered" "$installer" || {
    echo 'embed-source: install.sh snapshot differs from backend/container' >&2
    exit 1
  }
else
  chmod --reference="$installer" "$rendered"
  mv -f -- "$rendered" "$installer"
fi
