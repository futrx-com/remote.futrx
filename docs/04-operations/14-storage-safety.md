# Disk quotas, capacity and pool migration

`PROJECT_ROOT_DISK_QUOTA` sets the default root quota (20GiB). Each project's
explicit disk override wins; clearing it restores the default. The initial
20GiB budget reserves room for the standard tool image and builds while placing
a finite bound on root growth. It is an operator starting point, not a measured
requirement for every optional software manifest: measure your image and working
set and raise it when needed. Defaults are applied when an instance without a
quota next converges. Existing nonempty root quotas are retained.

Capability detection adapts the existing work in PR #48. It resolves each
instance's actual root pool, including an instance moved away from the default
profile's pool. ZFS/Btrfs/LVM/Ceph receive device quotas; unsupported drivers such
as `dir` are reported explicitly. Failed supported-driver quota application
blocks launch rather than starting an unbounded instance. Pool capability
inspection failure is shown as unavailable, not as a successful zero quota.

Containers → Info shows root usage/quota and a separate persistent-project total.
The latter samples the host directory holding `/workspace` and provider homes.
Sampling runs in the background, at most one scan at once, with a three-second /
250,000-entry budget and a five-minute cache. Incomplete scans report unknown,
not partial totals. Refresh after the first pending sample. The scan skips
symlinks and uses allocated blocks; copy-on-write shared extents and snapshots
mean these figures should not be added up as exclusive physical pool ownership.

Settings → Server information warns for individual filesystems at 80% byte or
inode usage. `DISK_WARNING_PERCENT` changes the threshold. The project panel also
warns for the filesystem actually holding its persistent data. Warnings recover
when the next sample falls below the threshold; they are status indicators, not
repeated push notifications. Unavailable inode metrics remain absent.

**A root quota does not constrain bind-mounted project data.** npm/Go caches in
`/workspace` also consume that host filesystem. Use a separately capacity-managed
filesystem/dataset for `/var/lib/remote/projects`, and apply per-project dataset
or filesystem quotas there if workloads must be unable to fill persistent
storage. Do not claim the root quota alone protects the entire host. Backups,
images and shared pool metadata require capacity planning too.

## Moving existing containers

First follow [storage setup and migration](11-storage.md) for driver/source
selection, compression, destination capacity, backups, instance inventory,
profiles, images and custom volumes. The operator must create the destination
pool; the helper never selects or formats a device.

During a maintenance window, stop new runs and stop the target container. Use a
backup filesystem with capacity for both the container export and persistent
project data. Run the preflight first:

```bash
bash infra/migrate-project-storage.sh \
  --instance remote-example --pool fast \
  --persistent-dir /var/lib/remote/projects/example \
  --backup-dir /mnt/backups/example-before-pool-move
```

Repeat with `--execute` to export the stopped instance, archive persistent data,
and run `lxc move <instance> --storage <pool>`. The supplied project path must
match its `/workspace` disk source. The script compares non-root mount devices
before and after and leaves the instance stopped. Backups are private and can
contain secrets. Custom application volumes outside the persistent directory
need their own backups; inventory them before invoking the helper.

After moving, start through Remote, inspect the effective quota and mount paths,
verify files and credentials, run both providers and installed applications, and
recreate a disposable migrated project. Change the default profile/installer
pool configuration for future projects as described in the storage guide. Keep
the source pool and backups until verification completes. A stopped instance
can be moved back to its original pool; if recovery needs the export, follow
LXD's import workflow without overwriting an instance that still holds data.

The commands follow the [official LXD instance storage migration guide](https://canonical.com/lxd/docs/latest/howto/storage_create_instance/).
Automated checks here mock LXD; a real move, quota exhaustion, compression and
recreation still need disposable-host QA before release.
