#!/usr/bin/env bash
# Package this image.
#
#   package.sh            rebuild container.tar.gz, the embedded payload
#   package.sh --zip [OUT] the same, then write an uploadable .zip
#
# go:embed does not traverse a nested Go module, and reports nothing when it
# skips one, so container/ reaches the server binary only as container.tar.gz.
# Run this after editing anything under container/.
#
# Both archives are reproducible — same source, same bytes — so a rebuild that
# changes no source leaves the working tree clean. Staleness of the payload is
# caught by TestS3diskPayloadMatchesContainerSource, which compares what is in
# it against container/ on disk.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
image_dir=$PWD

zip_out=
case ${1-} in
"") ;;
--zip)
  # Default outside the image directory on purpose: //go:embed images would
  # sweep a .zip left in here into the server binary — a copy of this folder,
  # inside this folder, inside every release.
  zip_out=${2-${TMPDIR:-/tmp}/s3disk.zip}
  if [ "$#" -gt 2 ]; then
    echo "package.sh: unexpected argument: $3" >&2
    exit 2
  fi
  ;;
*)
  echo "usage: package.sh [--zip [OUTPUT.zip]]" >&2
  exit 2
  ;;
esac

# --mtime, --owner/--group and --mode are what pin every field tar would
# otherwise take from the filesystem. bsdtar (macOS) spells them differently
# or not at all, so fail on the tar, not on a diff nobody can explain.
if ! tar --version 2>/dev/null | head -n 1 | grep -q 'GNU tar'; then
  echo "package.sh: GNU tar is required" >&2
  exit 1
fi

# A symlink would be excluded by -type f below and simply go missing from the
# archives. The catalog validator rejects links outright, so say so here.
if [ -n "$(find . -type l -print -quit)" ]; then
  echo "package.sh: symlinks are not package sources" >&2
  exit 1
fi

# The module's source and nothing else: no build outputs, caches, credentials
# or VCS metadata. This is the same set the payload test walks — keep them in
# step.
container_sources() {
  find container -type f \
    \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' -o -name 'VERSION' \) -print0
}

# LC_ALL=C sort fixes the member order; --no-recursion keeps tar to the listed
# files, so the archive holds no directory entries.
pack() {
  LC_ALL=C sort -z |
    tar --create --format=ustar \
      --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner --mode='0644' \
      --null --no-recursion --files-from=-
}

container_sources | pack | gzip -9 -n >container.tar.gz
echo "package.sh: wrote container.tar.gz ($(wc -c <container.tar.gz) bytes)"

[ -n "$zip_out" ] || exit 0

if ! command -v zip >/dev/null; then
  echo "package.sh: --zip needs the zip command (apt-get install zip)" >&2
  exit 1
fi
if [ -d "$zip_out" ]; then
  echo "package.sh: $zip_out is a directory" >&2
  exit 1
fi
out_dir=$(cd "$(dirname "$zip_out")" && pwd)
zip_out=$out_dir/$(basename "$zip_out")
case $out_dir/ in
"$image_dir"/*)
  echo "package.sh: refusing to write inside the image — //go:embed images" >&2
  echo "would carry it into the server binary. Pass a path outside it." >&2
  exit 1
  ;;
esac

# An uploaded package must name itself: a built-in image may inherit its id
# from its directory, an upload has no directory to inherit from. A shape
# check, not a parse — the server validates for real at upload.
for key in id version; do
  if ! grep -q "\"$key\"[[:space:]]*:" image.json; then
    echo "package.sh: image.json needs \"$key\" to be uploadable" >&2
    exit 1
  fi
done

# The archive is an images/<id>/ directory: everything a built-in image holds,
# including container.tar.gz — the server extracts a package as-is and never
# builds a payload itself. container/ rides along beside it because install.sh
# falls back to the source next to it when it was not staged from a payload.
zip_members() {
  find . -maxdepth 1 -type f ! -name '*.zip' -print0
  find plugin ui -type f -print0
  container_sources
}

# zip reads mtime and mode off the filesystem, so normalize a copy rather than
# the checkout. 2020 because the zip format cannot store a date before 1980.
stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT
zip_members | pack | tar --extract --directory "$stage"
find "$stage" -type d -exec chmod 755 {} +
find "$stage" -type f -exec chmod 644 {} +
find "$stage" -exec touch -t 202001010000 {} +

rm -f -- "$zip_out"
(cd "$stage" && find . -type f -printf '%P\n' | LC_ALL=C sort | zip -X -9 -q "$zip_out" -@)
echo "package.sh: wrote $zip_out ($(wc -c <"$zip_out") bytes)"
