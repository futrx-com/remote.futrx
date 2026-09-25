---
name: s3disk
description: "Work with files in a project's installed S3Disk mount: copy files into a requested bucket folder, flush writes, and check the result. Use for S3Disk storage tasks, not for changing the application's code or configuration."
---

# S3Disk storage

S3Disk mounts one configured S3 bucket, possibly under a prefix, inside this project's container. Files written beneath the mountpoint become objects under that bucket/prefix. The default mountpoint is `/workspace/s3`, but the installed copy may use another path.

## Find and verify the mount

1. Run `/usr/local/bin/s3disk list` to find the live mountpoint and source. If it lists none, check `systemctl is-active s3disk` and use the `s3disk-inspector` skill for diagnosis. Do not write into the unmounted directory: those files would stay on the container filesystem.
2. Run `findmnt -n -o SOURCE,TARGET --mountpoint "$mount_dir"`, with `mount_dir` set to the path from the list. Confirm the target is that mountpoint and the source starts with `s3disk#`. Use the same path in subsequent commands.

## Put files in a folder

Choose the relative folder requested by the user beneath the verified mountpoint. For example, with the default mountpoint, placing `/workspace/report.csv` in bucket folder `reports/2026` is:

```bash
mount_dir=/workspace/s3  # replace with the verified mountpoint
destination="$mount_dir/reports/2026"
mkdir -p -- "$destination"
cp -- /workspace/report.csv "$destination/"
/usr/local/bin/s3disk sync "$mount_dir"
test -f "$destination/report.csv"
```

Use `cp -R --` for a directory. Check for an existing destination before copying; follow the user's overwrite instructions rather than silently replacing a file. Keep the source until the copy and `s3disk sync` succeed. `sync` flushes pending writes to S3 and may take time for large files. Report the destination path and whether the flush completed; a read through the mount is not an independent check against S3.

Chat attachments may already be copied by S3Disk's browser extension into its configured uploads folder (default `uploads`). Use the attachment path supplied in the chat: on failure it can still be in `/workspace/.uploads`. Files created by an agent or CLI are not handled by that browser event, so copy those explicitly. `push` is a backend route for finished chat attachments, not a `s3disk` CLI command.

Do not print `/etc/remote/applications/s3disk/environment` or AWS credentials. For mount failures, use the `s3disk-inspector` skill; do not claim a file reached the bucket when the mount or flush failed.
