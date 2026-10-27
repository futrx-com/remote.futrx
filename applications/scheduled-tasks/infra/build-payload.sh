#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
    -cf - infra/remote-schedule | gzip -n > infra/payload.tar.gz
