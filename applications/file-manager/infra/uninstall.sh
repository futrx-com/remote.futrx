#!/usr/bin/env bash
set -euo pipefail
# Remote stops the service and owns its unit; the workspace belongs to the project.
rm -f /usr/local/bin/file-manager /usr/local/lib/remote/file-manager.build
