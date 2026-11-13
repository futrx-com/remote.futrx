# file-manager limitations

- **Directory and search limits are fixed constants.** Listings truncate at 10,000 entries per directory; search is filename-substring only (no content search), returns at most 300 results, and gives up after visiting 200,000 entries. In-browser preview is limited to an image/audio/video/PDF allowlist; other types can't be opened inline.
- **Archive downloads are capped.** Folder-ZIP downloads are limited to 1 GiB of source files each, with at most 2 concurrent builds per project; a larger folder fails with a message.
- **Finished ZIPs use workspace disk.** Each one waits in `/workspace/.file-manager-downloads/` (gitignored, hidden from the drawer) for up to an hour, until the next folder download removes it.
- **Escaping symlinks are silently omitted** from listings and downloads (a safety measure, but it can hide files).
