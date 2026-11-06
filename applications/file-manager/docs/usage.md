# File Manager usage

Install **File Manager** from the project's Applications tab before browsing,
or install it globally from **Settings → Applications** to add it to every
existing project. Repeat the global install for projects created later.
The Files action is available only in projects with a running installation.
Stopping or uninstalling the application removes the drawer and leaves workspace
files intact. Chat attachment previews and file links remain available.

![The Files drawer showing the durable project workspace](/assets/docs/screenshots/19-file-manager-14m10s.webp)

## Browse and download files

1. Open the project chat.
2. Select **Files** in the chat header.
3. Select a folder row to expand it. Subdirectories load as they are opened.
4. Select a file row to open it in the media viewer or an installed application,
   or to download it as described below.
5. Hover or focus a file and select **Download _filename_** to download
   directly instead.
6. To export a directory, hover or focus it and select **Download _folder_ as
   zip**.
7. Select **Refresh** after an agent or terminal command changes the tree.
8. Select **Close files** when finished.

**Outcome:** supported media opens inside Remote. Code, data, and text files
use a registered application opener when available; otherwise they download.
Folders download as ZIP archives.

### Search by filename

1. Open **Files**.
2. Enter at least two Unicode code points in **Search all files...**.
3. Select a matching file row to open it, or use its download control. Search
   result directories can also be expanded or downloaded.
4. If Remote says **Showing the first matches only — refine your search to
   narrow it down**, add a more specific substring.
5. Clear the search field to return to the directory tree.

Search matches path names by substring. It does not search file contents.

### Exact Files limits

| Operation | Current limit |
| --- | --- |
| One directory listing | 10,000 entries |
| Minimum search query | 2 Unicode code points |
| Returned search matches | 300 |
| Entries visited by one search | 200,000 |
| Folder ZIP source data | 1 GiB |
| Folder ZIP compressed size | 256 MiB |
| Entries in one folder ZIP | 200,000 |
| Folder ZIP build time | 5 minutes |
| Folder ZIP jobs | 2 concurrently per project File Manager installation |

A directory can therefore show a truncated warning even when its contents
exist. Search can stop at its visit cap before finding every possible match.
A ZIP fails when its source-byte, compressed-size, entry, or time limit is
reached. Single-file downloads have no File Manager size limit.

Folder archives omit broken symlinks, symlinks that escape the workspace, and
special files such as sockets or devices. They include regular files and safe
directories.

### What happens when you select a file

| File type | Selection result |
| --- | --- |
| Supported image, audio, video, or PDF | Opens in Remote's full-screen media viewer |
| Code, data, text, log, or unknown non-media file | Uses a registered application file opener, or downloads |
| Archive or unsupported image/audio/video format | Downloads the file |

The media viewer provides **Open in new tab**, **Download**, and **Close**.
Press Escape or select outside the content to close it.


## Download a deliverable

1. Ask the agent to put the final artifact in `/workspace`.
2. Choose **Files**.
3. Expand folders or use filename search.
4. Hover the file and choose download.
5. For a directory, choose the folder download action to receive a ZIP.

Folder archives are limited to 1 GiB of source files, a 256 MiB ZIP, and two concurrent downloads per project. For larger artifacts, use the terminal, Git, or an external object store.


## Feature reference

| Feature | How to use it | Limits or lifecycle |
| --- | --- | --- |
| Open Files | Choose **Files** | Lazy workspace tree |
| Refresh Files | Use the drawer refresh control | Reloads root |
| Expand folder | Select folder row | Lazy-loads children |
| Open file | Select a file or search result | Supported media opens in-app; an installed application can open source files, otherwise they download; unsupported media/archives download |
| Search filenames | Type at least two characters | 300 results; 200,000 visited-entry cap |
| Download file | Hover a file and choose download | Direct file response |
| Download folder | Hover a folder and choose download | ZIP up to 256 MiB from 1 GiB of files; two concurrent |
