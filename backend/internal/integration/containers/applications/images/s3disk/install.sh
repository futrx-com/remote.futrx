#!/usr/bin/env bash
# Idempotent s3disk provisioner. Runs as root inside the project's container.
#
# Contract (see applications.InstallSpec / installer.go):
#   - S3DISK_BUCKET           s3://bucket[/prefix] to mount              (required)
#   - S3DISK_MOUNTPOINT       where to mount it     (default /workspace/s3)
#   - AWS_ACCESS_KEY_ID       (required, secret)
#   - AWS_SECRET_ACCESS_KEY   (required, secret)
#   - AWS_REGION              (default us-east-1)
#   - S3DISK_ENDPOINT         non-AWS endpoint URL, optional
#   - S3DISK_MOUNT_ARGS       extra s3disk flags, optional
#
# APP_PACKAGE_DIR is supplied by Remote when staging the bundled payload.
# Running this script directly from the unpacked package is supported too.
#
# This is a "tool" image: nothing listens, so there is no APP_INTERNAL_PORT and
# no port to bind. The payload is a FUSE mount supervised by systemd, which is
# what the platform's stop/start/uninstall act on.
#
# Re-run on every install and every start, so every step is idempotent.
set -euo pipefail

# Default inside the project workspace on purpose. Agents run under a
# "workspace-write" sandbox, which permits writes only beneath /workspace: a
# mount at /mnt/s3 is readable but every write is refused by the agent's own
# policy, long before it reaches this filesystem.
MOUNTPOINT="${S3DISK_MOUNTPOINT:-/workspace/s3}"
UNIT="s3disk"
ENV_FILE="/etc/s3disk/${UNIT}.env"
IDLE_DIR="/etc/remote/workspace-idle.d"
BIN="/usr/local/bin/s3disk"
PACKAGE_DIR="${APP_PACKAGE_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
SOURCE_DIR="$PACKAGE_DIR/container"
GO_VERSION="1.24.7"

if [ ! -f "$SOURCE_DIR/go.mod" ] || [ ! -f "$SOURCE_DIR/VERSION" ]; then
  echo "install: the package is missing its bundled container source" >&2
  exit 1
fi
SOURCE_VERSION="$(cat "$SOURCE_DIR/VERSION")"
# A source edit must replace the installed executable even before VERSION is bumped.
SOURCE_HASH="$(cd "$SOURCE_DIR" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | cut -d' ' -f1)"
BUILD_VERSION="${SOURCE_VERSION}+${SOURCE_HASH:0:16}"

if [ -z "${S3DISK_BUCKET:-}" ]; then
  echo "install: S3DISK_BUCKET is required (e.g. s3://my-bucket/project)" >&2
  exit 1
fi
# Catch the endpoint-in-the-bucket-field mistake here, before anything is
# installed or a unit is written. Left to fail later it surfaces as an auth
# error against the wrong host, buried in a crash-looping unit.
case "$S3DISK_BUCKET" in
  s3://*) ;;
  *://*)
    echo "install: the bucket field contains a URL (${S3DISK_BUCKET})." >&2
    echo "         Put the bucket or storage-zone name here — 's3://my-bucket'" >&2
    echo "         or 's3://my-bucket/prefix' — and put ${S3DISK_BUCKET} in the" >&2
    echo "         'Endpoint URL' field instead." >&2
    exit 1
    ;;
esac
if [ -z "${AWS_ACCESS_KEY_ID:-}" ] || [ -z "${AWS_SECRET_ACCESS_KEY:-}" ]; then
  echo "install: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are required" >&2
  exit 1
fi
case "$MOUNTPOINT" in
  /|/etc|/usr|/var|/root|/home|/workspace)
    echo "install: refusing to mount over ${MOUNTPOINT}" >&2
    exit 1
    ;;
esac

export DEBIAN_FRONTEND=noninteractive
APT="apt-get -o DPkg::Lock::Timeout=300"

# --- FUSE userspace ---------------------------------------------------------
# fusermount3 is what unmounts the filesystem; ca-certificates is what lets it
# talk to S3 over TLS.
if ! command -v fusermount3 >/dev/null 2>&1; then
  $APT update -qq
  $APT install -y -qq --no-install-recommends fuse3 ca-certificates
fi

# --- the s3disk binary ------------------------------------------------------
install_from_source() {
  # Build in the container so the result matches its architecture. The Go
  # toolchain is only fetched when it is missing.
  local goroot="/usr/local/go" arch
  if ! "${goroot}/bin/go" version >/dev/null 2>&1; then
    arch="$(dpkg --print-architecture)"
    case "$arch" in
      amd64|arm64) ;;
      *) echo "install: no Go toolchain available for architecture ${arch}" >&2; return 1 ;;
    esac
    if ! command -v curl >/dev/null 2>&1; then
      $APT update -qq && $APT install -y -qq --no-install-recommends curl
    fi
    echo "install: fetching the Go toolchain to build s3disk"
    curl -fsSL -o /tmp/go.tgz \
      "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" || return 1
    rm -rf "$goroot"
    tar -C /usr/local -xzf /tmp/go.tgz
    rm -f /tmp/go.tgz
  fi
  echo "install: building bundled s3disk ${BUILD_VERSION}"
  local build_dir
  build_dir="$(mktemp -d)"
  if ! (cd "$SOURCE_DIR" && CGO_ENABLED=0 "${goroot}/bin/go" build \
      -mod=readonly -buildvcs=false -trimpath \
      -ldflags "-s -w -X main.version=${BUILD_VERSION}" \
      -o "$build_dir/s3disk" ./cmd/s3disk); then
    rm -rf "$build_dir"
    return 1
  fi
  if ! install -m 0755 "$build_dir/s3disk" "$BIN"; then
    rm -rf "$build_dir"
    return 1
  fi
  rm -rf "$build_dir"
}

installed_version() { "$BIN" version 2>/dev/null | awk '{print $2}'; }
if [ "$(installed_version || true)" != "$BUILD_VERSION" ]; then
  install_from_source || {
    echo "install: could not build the bundled s3disk source" >&2
    exit 1
  }
fi
"$BIN" version

# --- credentials ------------------------------------------------------------
# Secrets live only here, readable by root, and never in the unit file (which
# is world-readable) or in this script's output.
mkdir -p "$(dirname "$ENV_FILE")"
umask 077
cat >"$ENV_FILE" <<EOF
AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID}
AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY}
AWS_REGION=${AWS_REGION:-us-east-1}
EOF
chmod 600 "$ENV_FILE"
umask 022

# --- the mount, supervised by systemd ---------------------------------------
MOUNT_ARGS="${S3DISK_MOUNT_ARGS:-}"
if [ -n "${S3DISK_ENDPOINT:-}" ]; then
  MOUNT_ARGS="--endpoint ${S3DISK_ENDPOINT} ${MOUNT_ARGS}"
fi
# This container owns its bucket or prefix, so it is the only writer: cached
# metadata never needs re-checking. Overridable through S3DISK_MOUNT_ARGS.
case "$MOUNT_ARGS" in
  *--exclusive*|*--no-exclusive*) ;;
  *) MOUNT_ARGS="--exclusive ${MOUNT_ARGS}" ;;
esac

mkdir -p "$MOUNTPOINT"

# Overwriting a file this image owns is idempotent; appending would not be.
cat >"/etc/systemd/system/${UNIT}.service" <<EOF
# Managed by the s3disk application image. Regenerated on every install/start.
[Unit]
Description=s3disk mount of ${S3DISK_BUCKET} at ${MOUNTPOINT}
After=network-online.target
Wants=network-online.target

[Service]
Type=exec
EnvironmentFile=${ENV_FILE}
ExecStart=${BIN} mount ${S3DISK_BUCKET} ${MOUNTPOINT} --foreground ${MOUNT_ARGS}
# s3disk turns SIGTERM into "upload everything still pending, then detach", so
# the stop timeout has to cover the largest outstanding upload.
# A normal stop reaches the main process first, which unmounts and exits, so
# this then finds nothing mounted. Tolerate that: the unit must not end up
# in the failed state after a clean stop.
ExecStop=-${BIN} umount ${MOUNTPOINT}
TimeoutStopSec=300
Restart=on-failure
RestartSec=5s
# Clear a dead mount, which would otherwise block every reader in the project.
ExecStopPost=-/bin/sh -c 'mountpoint -q ${MOUNTPOINT} && fusermount3 -uz ${MOUNTPOINT}'

[Install]
WantedBy=multi-user.target
EOF

# The mount daemon runs for the life of the container. Remote's idle-workspace
# probe treats any unrecognised process as someone working in the project, so
# declare this one: without it, installing s3disk would pin every workspace it
# is installed in and workspace backups would never archive an idle project.
# Remote reads names from this directory; it ships no list of its own, and
# it matches on /proc/<pid>/comm, so the executable's name is what goes in.
mkdir -p "$IDLE_DIR"
printf '%s\n' "$(basename "$BIN")" >"${IDLE_DIR}/${UNIT}"

systemctl daemon-reload
systemctl enable "$UNIT" >/dev/null 2>&1 || true
systemctl restart "$UNIT"

# --- wait until the filesystem is actually usable ---------------------------
# "Something is mounted here" is not the test. A mount left behind by another
# s3disk — an earlier bucket, or one started by hand — satisfies mountpoint(1)
# while this unit is dead, so the install would report success and every write
# would go to the wrong bucket. Wait for the source this unit mounts instead.
# s3fs names it "s3disk#<bucket>[/<prefix>]"; see internal/s3fs/mount.go.
EXPECTED_SOURCE="s3disk#${S3DISK_BUCKET#s3://}"
EXPECTED_SOURCE="${EXPECTED_SOURCE%/}"

# With mounts stacked on one directory the last line is the one readers reach.
mounted_source() {
  # findmnt exits non-zero when nothing is mounted, which under pipefail
  # would abort the script before it can report why.
  findmnt -no SOURCE "$MOUNTPOINT" 2>/dev/null | tail -n 1 || true
}

for _ in $(seq 1 30); do
  if [ "$(mounted_source)" = "$EXPECTED_SOURCE" ]; then
    break
  fi
  sleep 1
done
if [ "$(mounted_source)" != "$EXPECTED_SOURCE" ]; then
  echo "install: s3disk did not mount at ${MOUNTPOINT}" >&2
  found="$(mounted_source)"
  if [ -n "$found" ]; then
    echo "install: ${MOUNTPOINT} is held by ${found}; unmount it and install again" >&2
  fi

  # Whatever s3disk logged is the reason; the unit's exit code is not. Read the
  # journal, but never trust it to have anything: journalctl exits 0 with no
  # output when a unit logged nothing, which would leave this reporting an
  # empty "log" and the user no better off.
  log="$(journalctl -u "$UNIT" --no-pager -n 25 -o cat 2>/dev/null || true)"
  if [ -n "$log" ]; then
    echo "install: --- s3disk log ---" >&2
    printf '%s\n' "$log" >&2
  fi

  # Ask s3disk itself what is wrong. This works even when nothing was logged,
  # and it checks the three things that actually fail: FUSE, credentials, and
  # whether the bucket answers at the configured endpoint.
  echo "install: --- s3disk doctor ---" >&2
  DOCTOR_ARGS=""
  [ -n "${S3DISK_ENDPOINT:-}" ] && DOCTOR_ARGS="--endpoint ${S3DISK_ENDPOINT}"
  case "$MOUNT_ARGS" in
    *--path-style*) DOCTOR_ARGS="$DOCTOR_ARGS --path-style" ;;
  esac
  # shellcheck disable=SC2086
  "$BIN" doctor $DOCTOR_ARGS "$S3DISK_BUCKET" 2>&1 | tail -20 >&2 || true

  exit 1
fi

echo "install: s3disk mounted ${S3DISK_BUCKET} at ${MOUNTPOINT}"
