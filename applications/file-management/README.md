# File Management

File Management is the built-in workspace browser application. It contributes a
**Files** pane to chats and uses its host backend to list and search the chat's
workspace, preview supported media, download individual files, and build folder
ZIP archives.

## Install and use

Remote installs and starts File Management globally once when a server first
upgrades to a release that includes it. An administrator can stop or uninstall
that copy without a later restart restoring it. The application may also be
installed in a project to limit a separate copy to that project's chats. Open a
chat and select **Files** in the workspace header. A project install is preferred
on that project's chats; otherwise the running global install is used.

The pane loads one directory level at a time. Search begins after two characters.
Selecting an image, video, audio file, or PDF opens Remote's media viewer;
selecting code or text opens the workspace IDE; archives and unsupported media
download instead. The download control on every row always downloads, and
folders download as ZIP files.

The backend only accepts chat-scoped calls. Remote authorizes the chat first and
stamps its trusted workspace root into `Request.Context.Chat`; the browser never
supplies a host path. Project chats resolve that root from their project record;
loose chats use Remote's fixed host installation workspace. A mutable chat
working directory cannot select `/`, another project, or another host path.
Filesystem operations are rooted beneath that directory, resolve symlinks
without allowing escapes, and refuse special files.

## Verify

1. Create a workspace with a nested directory, a dotfile, a text file, an image,
   and a ZIP archive.
2. Open **Files**, expand the directory, refresh, and search by a two-character
   substring.
3. Confirm the image opens in the viewer, the text file opens in the IDE, and
   the ZIP downloads.
4. Download a directory and inspect the ZIP. A symlink leaving the workspace
   must neither appear in the pane nor enter the archive.
5. Stop the application and confirm its pane disappears; start it and confirm
   the pane returns.

## Limits

- One directory listing returns at most 10,000 entries.
- Search returns at most 300 matches and visits at most 200,000 entries.
- A ZIP may read at most 1 GiB of source data, contain at most 200,000 entries,
  and produce at most a 1 GiB spool file. At most two ZIPs are building or
  being transferred across all global and project installs on one server, so
  active spool data stays at or below roughly 2 GiB in aggregate.
- The five-minute backend timeout bounds opening a response and building a ZIP;
  once its response stream is open, the HTTP request owns the transfer lifetime.

## Upgrade and cleanup

Changing the manifest version makes installed copies reconverge on the new
package. The application has no container resources, service, port, credentials,
or install script. Its only persistent state is temporary ZIP data beneath the
instance `DataDir`; completed and failed requests remove their spool files, and
uninstall removes the instance data directory. The backend holds one of two
server-wide archive locks in the application's `SharedRuntimeDir` from the start
of ZIP construction until the response stream closes. Process exit releases the
lock automatically, and Remote removes shared runtime files during package
replacement and server shutdown.

File, ZIP, and media bodies use the platform's seekable response stream. Remote
serves byte ranges and conditional requests and closes the application-owned
reader when the transfer completes or the browser disconnects. Folder ZIPs are
still built into a bounded spool file before a success response is opened, so a
late archive error can be reported cleanly instead of truncating a `200` body.

## Package layout

- `backend/main.go` composes the backend.
- `backend/api/` owns routes and response translation.
- `backend/workspace/` owns rooted filesystem access, policy, media types, and
  bounded ZIP spooling.
- `ui/scripts/` owns pane state, file policy, and DOM rendering.
- `ui/style/` contains namespaced, token-based plain CSS.
