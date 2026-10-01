# Persistent workspace and provider-home quotas

A project's LXD root quota does not include its host bind mounts. The optional
persistent-storage backend creates a separate ZFS child dataset for each project
at the existing `/var/lib/remote/projects/<slug>` path. Its quota covers
`workspace`, `agent-home`, npm/Go caches and any other files below that project
path. Container recreation keeps the same dataset and quota.

## Configure a host

Use an operator-owned ZFS parent dataset mounted exactly at
`/var/lib/remote/projects`; it must not be an LXD-managed internal dataset. On a
fresh installation, first verify the directory is empty and no projects or
containers use it. Then create the parent on an existing, explicitly selected
pool. This example does not create a pool or select/format a device:

```bash
sudo zfs create -o mountpoint=/var/lib/remote/projects -o compression=lz4 \
  -o overlay=off tank/remote-projects
```

Set these backend environment variables using the server's systemd environment
configuration, then restart the backend during a maintenance window:

```text
PROJECT_STORAGE_DATASET=tank/remote-projects
PROJECT_PERSISTENT_DISK_QUOTA=20GiB
PROJECT_PERSISTENT_QUOTA_REQUIRED=true
```

The parent dataset must already be mounted. Backend privileges must permit ZFS
list/get/create/set and mountpoint verification using `findmnt`. The parent and
per-project mountpoint directories must remain host-owned; never delegate ZFS
administration to container users.

Each newly provisioned project gets a child dataset with compression and a
20GiB quota by default. `PROJECT_PERSISTENT_DISK_QUOTA` accepts positive MiB,
GiB or TiB values. An existing finite project quota is preserved; administrators
can adjust it using `zfs set quota=<size> <dataset>`. A zero/unlimited quota is
replaced with the configured default at the next lifecycle convergence.

With a dataset configured, unavailable tools, mismatched mounts, unknown state
or quota failures block container convergence. `PROJECT_PERSISTENT_QUOTA_REQUIRED`
can also require enforcement when the dataset configuration is missing. Its
literal `false` default preserves legacy installations; any other value requires
enforcement. Legacy directory/Btrfs persistent storage is shown as **Not enforced**,
not falsely protected by the independent container-root quota. This adapter
supports ZFS persistent quotas; existing LXD root quotas still support the drivers
listed in the disk-safety guide.

Containers → Info shows the effective persistent quota and whether enforcement
was verified. It is read back from ZFS and the actual mounted filesystem, not
inferred from a saved setting. The normal usage scan remains bounded/cached.

## Migrate existing persistent project data

Do not enable the dataset configuration over a nonempty project tree. Remote
refuses to create a mount over any existing ordinary project directory. During
an explicit maintenance window:

1. Stop new runs, Remote, and every affected container. Back up project metadata,
   the entire persistent tree, and separately mounted application volumes to a
   different filesystem; verify restoration before proceeding.
2. Move the old tree aside while it is offline. Create/mount the parent dataset
   at the canonical path with `overlay=off`; never mount over the only copy.
3. For each project, create a child dataset at its original slug path with a
   quota large enough for its actual data. Set `remote:project-id` to the exact
   immutable project ID in that project's backed-up `meta.json`. For example:
   `zfs create -o quota=20G -o compression=lz4 -o overlay=off -o remote:project-id=abcd1234 -o mountpoint=/var/lib/remote/projects/example tank/remote-projects/example`.
4. Restore the whole project directory, including `agent-home`, numeric ownership,
   ACLs and extended attributes. Keep the mountpoint host-owned. Compare data,
   metadata and byte counts against the backup. Do not copy only `/workspace`.
5. Configure the backend, start a disposable migrated project, verify quota
   reporting, provider authentication, application data and recreation. Retain
   the backup and old directory tree through acceptance. Rollback requires
   stopping the same services, unmounting the new datasets and restoring the
   old tree at its original path; do not destroy the new datasets first.

A retained dataset cannot be adopted by a different project reusing the same
slug: the immutable `remote:project-id` must match. Project deletion does not
destroy persistent datasets or snapshots. Inspect and archive them before any
explicit administrator cleanup; retained storage still consumes pool capacity.

ZFS `quota`, unlike `refquota`, includes descendant datasets and snapshots; see
the [OpenZFS quota property](https://openzfs.github.io/openzfs-docs/man/master/7/zfsprops.7.html#quota).
These limits cap each project's consumption; they do not reserve capacity or
prevent the sum of many projects/backups/images from filling a pool. Retain the
existing host capacity checks and warnings.

Unit tests simulate ZFS command results and mount verification. Real quota
exhaustion and LXD migration/rollback also passed on an isolated Ubuntu guest;
see the [validation record](15-requirements-validation.md). Concurrent real
provider authentication and full Remote image recreation still need acceptance
on a configured QA deployment. The ordinary suite never mutates its host.

For an explicit real-filesystem check, create a disposable parent dataset ending
in `/remote-qa`, mounted at a separate disposable directory, then run the opt-in
`TestZFSQuotaOnDisposableHost` test with `REMOTE_ZFS_QA_DATASET` and
`REMOTE_ZFS_QA_ROOT`. It uses a 32MiB project quota, verifies an actual write is
rejected at exhaustion, checks provider-home data survives manager recreation,
and rejects a different project identity. It never creates/destroys a pool and
retains its small test datasets for inspection. Do not point it at production.

## Disposable LXD root-volume fixture

The opt-in `TestRootQuotaOnDisposableLXD` uses an instance named exactly
`remote-migrate-qa` on a ZFS LXD pool. Build the tiny static init/probe from the
backend module with `CGO_ENABLED=0 go build -o qa-init ./internal/integration/containers/resources/testdata/init`.
Package it as both `rootfs/sbin/init` and `rootfs/bin/qa` in a minimal LXD image,
with ordinary empty `etc`, `proc`, `sys`, `dev`, `run`, `tmp`, `root` and `workspace`
directories and an `x86_64` metadata.yaml. This image is only a filesystem test
fixture, not a usable Remote base image. The probe supports bounded writes and
reads without needing an Ubuntu installation or provider credentials.

Set `REMOTE_LXD_QA_INSTANCE=remote-migrate-qa` when running that test. It invokes
Remote's actual resource manager, verifies the resolved pool and 64MiB limit,
expects a real 128MiB attempted write to fail with a storage/quota error, and
removes the disposable probe file. It does not delete the instance or its pool.
