# File Manager troubleshooting

### File search finds nothing

Search is filename substring search, not content search. Use at least two characters. Results cap at 300 after visiting at most 200,000 entries. Very large individual directory listings cap at 10,000 entries.

### Selecting a file downloads it instead of opening it

Archives and unsupported image, audio, or video formats deliberately fall back
to download. Supported image/audio/video/PDF files open in Remote's media
viewer; code, data, text, logs, and other non-media files open in the project
IDE. Use the explicit download icon when you want the local file regardless of
type.

### Folder download fails

Folder ZIP downloads are limited to 1 GiB of source files, five minutes to build, and two simultaneous archives per project. The reason appears in a banner above the file list. Symlinks that escape the project path are omitted. Use Git, Terminal, or external storage for a larger export.

If the banner says File Manager is out of date, the project's container still
has the program from an earlier build. Remote reinstalls outdated copies when it
starts and when a project container starts, so this lasts only until that
reinstall finishes; if it persists, check the server log for a failed upgrade,
then stop and start File Manager for that project and download again.

