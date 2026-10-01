#!/usr/bin/env bash
# Offline, operator-invoked root-volume migration. Never creates/wipes a pool.
set -euo pipefail
umask 077
instance="" pool="" backup_dir="" persistent_dir="" execute=""
while (($#)); do
    case "$1" in
        --instance|--pool|--backup-dir|--persistent-dir)
            [ "$#" -ge 2 ] || { echo "Missing argument" >&2; exit 2; }
            case "$1" in
                --instance) instance=$2;;
                --pool) pool=$2;;
                --backup-dir) backup_dir=$2;;
                --persistent-dir) persistent_dir=$2;;
            esac
            shift 2;;
        --execute) execute=1; shift;;
        *) echo "Usage: $0 --instance NAME --pool POOL --backup-dir NEW_DIR --persistent-dir PROJECT_PARENT [--execute]" >&2; exit 2;;
    esac
done
[[ "$instance" =~ ^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$ ]] || { echo "Invalid instance" >&2; exit 2; }
[[ "$pool" =~ ^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$ ]] || { echo "Invalid target pool" >&2; exit 2; }
[[ "$backup_dir" = /* && "$persistent_dir" = /* && "$persistent_dir" != / ]] || { echo "Use absolute backup and project paths" >&2; exit 2; }
[ -d "$persistent_dir/workspace" ] || { echo "Persistent project parent must contain workspace/" >&2; exit 2; }
[ ! -e "$backup_dir" ] || { echo "Backup directory must be new" >&2; exit 2; }
backup_dir=$(realpath -m "$backup_dir")
persistent_dir=$(realpath "$persistent_dir")
case "$backup_dir/" in "$persistent_dir/"*) echo "Backup must be outside project data" >&2; exit 2;; esac
command -v jq >/dev/null
command -v lxc >/dev/null
instance_json=$(lxc query "/1.0/instances/$instance")
[ "$(jq -r '.status' <<<"$instance_json")" = Stopped ] || { echo "Stop the instance and disable workload admission before migration" >&2; exit 1; }
source_pool=$(jq -r '.expanded_devices | to_entries[] | select(.value.type=="disk" and .value.path=="/") | .value.pool' <<<"$instance_json")
[ -n "$source_pool" ] && [ "$source_pool" != "$pool" ] || { echo "Source and target pools must differ" >&2; exit 1; }
# Verify the supplied durable workspace is the actual mounted source.
actual_workspace=$(jq -r '.expanded_devices | to_entries[] | select(.value.type=="disk" and .value.path=="/workspace") | .value.source' <<<"$instance_json")
[ "$(realpath "$persistent_dir/workspace")" = "$(realpath "$actual_workspace")" ] || { echo "Persistent path does not match instance workspace" >&2; exit 1; }
pool_json=$(lxc query "/1.0/storage-pools/$pool")
case "$(jq -r '.driver' <<<"$pool_json")" in zfs|btrfs) ;; *) echo "Destination must use ZFS or Btrfs" >&2; exit 1;; esac
printf 'Plan: back up %s and its persistent data, then move root storage from %s to %s. Instance remains stopped.\n' "$instance" "$source_pool" "$pool"
[ -n "$execute" ] || { echo "Preflight only; repeat with --execute during the maintenance window."; exit 0; }
mkdir -m 700 -- "$backup_dir"
printf '%s\n' "$instance_json" > "$backup_dir/instance-before.json"
printf '%s\n' "$pool_json" > "$backup_dir/target-pool.json"
lxc storage show "$source_pool" > "$backup_dir/source-pool.yaml"
lxc config show "$instance" --expanded > "$backup_dir/instance-expanded.yaml"
lxc export "$instance" "$backup_dir/instance.tar.gz"
tar --numeric-owner --acls --xattrs -czpf "$backup_dir/persistent.tar.gz" -C "$persistent_dir" .
# A failed export/archive exits before the source instance is changed.
lxc move "$instance" --storage "$pool"
lxc query "/1.0/instances/$instance" > "$backup_dir/instance-after.json"
jq -e --arg pool "$pool" '.expanded_devices | any(.[]; .type=="disk" and .path=="/" and .pool==$pool)' "$backup_dir/instance-after.json" >/dev/null
jq -S '.expanded_devices | with_entries(select(.value.path!="/"))' "$backup_dir/instance-before.json" > "$backup_dir/mounts-before.json"
jq -S '.expanded_devices | with_entries(select(.value.path!="/"))' "$backup_dir/instance-after.json" > "$backup_dir/mounts-after.json"
cmp -s "$backup_dir/mounts-before.json" "$backup_dir/mounts-after.json" || { echo "Mount verification failed; keep stopped and use the backup for recovery." >&2; exit 1; }
printf 'Moved and verified mounts. Keep backups and source pool until workload verification. Roll back while stopped: lxc move %s --storage %s\n' "$instance" "$source_pool"
