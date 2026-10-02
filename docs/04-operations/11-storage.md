# Workspace storage selection

Fresh installations accept these flags (or the corresponding environment
variables):

| Flag | Environment | Default |
| --- | --- | --- |
| `--storage-driver` | `FUTRX_STORAGE_DRIVER` | `auto` |
| `--storage-pool` | `FUTRX_STORAGE_POOL` | `default` |
| `--storage-source` | `FUTRX_STORAGE_SOURCE` | LXD-managed source |
| `--storage-size` | `FUTRX_STORAGE_SIZE` | LXD's loop-size default |

For example, a dedicated host with ZFS support can create a loop-backed pool:

```bash
sudo bash infra/install.sh remote.example.com \
  --storage-driver=zfs --storage-pool=workspaces --storage-size=100GiB
```

For an operator-prepared source, use `--storage-source` instead of
`--storage-size` and specify the driver explicitly. The source is passed to LXD
as one JSON string, never interpreted as shell code. LXD may initialize the
supplied device: select only storage dedicated to Remote and verify backups
before supplying a block-device path. The installer does not discover or select
a device automatically and never enables `source.wipe`.

`auto` queries LXD's reported supported drivers, preferring ZFS, then Btrfs.
If neither is reported, it warns and uses `dir`. Nested unprivileged LXC uses
`dir` in automatic mode because tool/kernel detection alone does not establish
permission to create pools there; explicitly configure a supported Btrfs source
when the outer container provides one. Install the selected driver's host
prerequisites before running the installer. An explicit unavailable driver or
a failed pool creation stops installation; it does not silently retry as dir.

The installer uses LXD's compression-on default for new ZFS pools. For Btrfs it
sets `btrfs.mount_options=user_subvol_rm_allowed,compress=zstd`; these mount
options apply to block-backed pools. For an existing Btrfs subvolume source,
configure and verify compression on its owning filesystem separately. Consult
the [ZFS driver reference](https://canonical.com/lxd/docs/latest/reference/storage_zfs/)
and [Btrfs driver reference](https://canonical.com/lxd/docs/latest/reference/storage_btrfs/).
Loop-backed pools share the host filesystem's capacity. Set their size
intentionally; LXD's default may be too small for many workspaces.

Rerunning the installer preserves existing storage. It requires a root disk on
the default profile and the `lxdbr0` bridge. Explicit flags that conflict with
an existing pool cause an error rather than changing that pool. Partially
configured installations must finish their LXD setup first. These flags are not
a migration or resize mechanism.

## Migrating an existing dir pool

Perform this as a planned maintenance operation on the host. Test on a disposable
installation first. See LXD's [pool migration instructions](https://canonical.com/lxd/docs/latest/howto/storage_move_volume/)
and [instance storage instructions](https://canonical.com/lxd/docs/latest/howto/storage_create_instance/).

1. Inventory the current pools, profiles, instances, images, snapshots and custom
   volumes. Record each instance's expanded devices and local root-device
   overrides. Record all host bind-mount source directories.
2. Back up Remote's data directory, host-mounted workspaces and provider homes
   with the backend and active workloads stopped. Export containers as needed;
   a container export is not a backup of its host bind mounts. Verify a restore.
3. Create a destination pool using an explicitly chosen source with sufficient
   free capacity, leaving the source pool intact. For a test-sized loop pool:

   ```bash
   lxc storage create workspaces-zfs zfs size=100GiB
   ```

4. Stop the Remote backend and each instance being moved. Move one instance at
   a time while preserving its name:

   ```bash
   lxc stop PROJECT_CONTAINER
   lxc move PROJECT_CONTAINER --storage=workspaces-zfs
   ```

   Inspect its expanded root device afterward; it must reference the new pool.
   Move any application-owned custom volumes separately and preserve their
   attachment paths. Do not move host bind mounts by changing root storage.
5. Update the default profile's root pool for future launches only after
   accounting for all instances inheriting it. Review other profiles and
   container-local root overrides as well:

   ```bash
   lxc profile device set default root pool workspaces-zfs
   ```

6. Restart the backend and representative workspaces. Verify files, ownership,
   provider sign-in, a real agent run, IDE/browser access and installed
   applications. Verify new project creation and image availability in the new
   pool. Measure pool/host space and compression behavior.
7. Keep backups and the old pool until checks pass. A move removes the original
   instance root volume; retaining the old pool alone is not a rollback copy.
   To roll back, stop the instance, move it back to its original pool, restore
   recorded profile/device settings, and restore backups if necessary. Remove
   old images/pools only after explicit review of their remaining dependencies.

This PR introduces storage selection, not fleet quota policy or disk warnings.
Those must build on the existing resource-control work. Root quotas cannot limit
Remote's host-mounted `/workspace` and provider homes; measure and bound those
on the host filesystem separately. Btrfs qgroups also have limitations documented
by LXD. Do not present a root quota as a complete per-project disk safety limit.

## Validation before release

On disposable supported hosts, run fresh installation and full update QA. Check
both a clean host and an existing dir installation; verify explicit sources,
compression, new/existing project launches, recreation and failed initialization.
Unit tests mock the LXD commands and cannot establish real filesystem behavior.
