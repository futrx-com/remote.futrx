#!/usr/bin/env bash
# Rebuild after editing an installer asset. Run from any working directory.
set -euo pipefail
cd "$(dirname "$0")/.."
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
    -cf - infra/remote-open.html infra/migrate-settings.cjs | gzip -n > infra/payload.tar.gz
