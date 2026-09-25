# S3Disk for Remote

Project-scoped application that mounts an S3 bucket inside the project container and provides a **Mount controls** action. It can also copy completed chat uploads into the mounted bucket.

## Install

Upload the ZIP in **Settings → Applications**, then install S3Disk in the project's Applications tab. Set `S3DISK_BUCKET` to `s3://bucket` or `s3://bucket/prefix`, provide the AWS access key ID and secret, and choose an optional endpoint for S3-compatible storage. The mount defaults to `/workspace/s3` so project agents can write to it.

For an uploaded ZIP, Remote builds `backend/container/cmd/s3disk` before running `infra/install.sh`. For the built-in catalog, Go omits S3Disk's nested Go module from the embedded files, so the shell installer builds the same pinned source snapshot when the binary is missing. Run `bash infra/embed-source.sh` after changing `backend/container/`; `bash infra/embed-source.sh --check` verifies the snapshot. The installer also installs FUSE and creates the mountpoint. Remote writes and starts the systemd service declared in `application.json`, including its base64-mapped settings and idle lifecycle. The installer writes no unit or credential file.

The host composition root in `backend/main.go` wires the request handlers in `backend/api/` to the per-instance mount operation lifecycle and push publisher in `backend/lifecycle/`. The API exposes mount status, diagnostics, sync, restart, operation progress, and attachment-copy actions to the UI. The project skills `s3disk` and `s3disk-inspector` guide agents through copying files to the mount and checking its health.

## Lifecycle events

| Publisher | Event | Owner | When |
|---|---|---|---|
| `applications.s3disk.push` | `completed` | S3Disk | A chat attachment push request finishes, including partial results. |
| `remote.applications` | `stopped` | Remote | A running S3Disk instance finishes stopping and its stopped state is saved. |

The push event carries `directory`, `requested`, `stored`, `removed`, and `issues` counts. It omits file names and credentials. With asynchronous writeback, `stored` can include a file that has not yet reached S3; its issue is counted, and the original remains in `.uploads`. A publication failure adds a warning to the push response without undoing the copy.

To receive stop events, another running application subscribes to `remote.applications` / `stopped` and filters the event payload for `applicationId: "s3disk"` and, if needed, `instanceId`. S3Disk cannot emit or receive its own application stop event after Remote has terminated its backend. Both event streams are in-memory and have no replay.

## Verify

Inside the project container, run `systemctl is-active s3disk` and `findmnt -no SOURCE /workspace/s3` (or your configured mountpoint). The source should be `s3disk#bucket[/prefix]`. `s3disk status /workspace/s3` shows cache and request counters. If startup fails, inspect `journalctl -u s3disk -n 100 --no-pager` and run `s3disk doctor s3://bucket` with the same endpoint options.

## Upgrade and stop

Version `0.5.2` restored the missing container binary during install, including when the application is built into Remote's catalog. Version `0.5.3` added the `s3disk` project skill, and `0.5.4` declares the push event. Uploading a new package version reconverges installed copies according to Remote's upgrade policy. The mount daemon attempts to flush writes before it stops. Remote's generated unit uses the systemd default stop timeout, so sync large pending writes with the Mount controls action before stopping or upgrading the application.

The mount uses `--exclusive` by default. Set `S3DISK_MOUNT_ARGS` to `--no-exclusive` to allow other writers, or add other S3Disk mount flags. Quoted values with spaces are supported. Remote keeps all service settings in its root-readable environment file under `/etc/remote/applications/s3disk/`.
