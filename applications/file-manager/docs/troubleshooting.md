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

Folder ZIP downloads are limited to 1 GiB of source files, a 256 MiB ZIP, five minutes to build, and two simultaneous archives per project. Symlinks that escape the project path are omitted. Use Git, Terminal, or external storage for a larger export.

