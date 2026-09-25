---
name: s3disk-inspector
description: Inspect and verify the installed S3disk mount and service from its project container.
---

# S3disk Inspector

Verify the installed project mount from inside its container.

1. Run `systemctl is-active s3disk` and report the exact state.
2. Run `/usr/local/bin/s3disk version` to confirm the installed command.
3. Run `/usr/local/bin/s3disk list` to identify the active mountpoint, then
   `/usr/local/bin/s3disk status <mountpoint>` to read mount and writeback state.
4. Run `findmnt -no SOURCE,TARGET <mountpoint>` and confirm its source starts
   with `s3disk#` and its target is the expected mountpoint. Do not treat an
   ordinary directory as a mounted bucket.
5. If a check fails, inspect `systemctl status s3disk --no-pager --full` and
   `journalctl -u s3disk -n 100 --no-pager` before suggesting a fix. Summarize
   errors without exposing credentials or unrelated file names.

Never print `/etc/remote/applications/s3disk/environment`: it contains encoded S3 credentials. Do not run
`s3disk doctor` without the user's request; it contacts the configured store.
