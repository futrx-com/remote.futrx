# S3disk

S3disk is a **built-in application image**: it ships in Remote's embedded
catalog and is versioned with the codebase. Being built in is only about where
it lives. No Go code in Remote names it, imports it or requires it; it loads
through the same generic validator an uploaded `.zip` goes through, and a host
that never installs it downloads nothing extra and keeps exactly the software it
was provisioned with. Everything specific to this application — its icon, its
host dependency — lives in this folder.

It combines a container tool with a Go backend and a browser extension:

- `container/` contains the complete standalone Go mount implementation, its
  module checksums, version and tests. This is the source to edit.
- `install.sh` builds that bundled source and supervises its FUSE mount with
  the container's `s3disk.service`. It never fetches the separate S3disk repo.
- `plugin/` is compiled by Remote, like Backend Playground. It runs on the host
  and uses the local `lxc exec` CLI to reach only the installed container.
- `ui/` owns the **Mount controls** action. Remote renders it through a
  generic extension slot only while the plugin is running in that project.
- `image.json` owns the bucket, endpoint, region, and AWS credential fields
  used by the generic application installation form. It also declares the two
  things this package needs from the host, described under *What this package
  asks the host for* below.

The backend exposes `GET status`, `GET diagnostics`, `GET operation`,
`POST sync`, `POST restart`, and `POST push`. Diagnostics include the executable version,
FUSE device and recent service journal; they do not perform a separate S3
connectivity test. Configured credentials are redacted from command output.
No route accepts a command, container name or mountpoint from the caller.
`push` accepts file names and nothing else — not paths — because both
directories it works between belong to this image. Existing project access
checks protect these routes.

Sync and restart return 202 immediately. Poll `operation` for completion;
only one such action can run per instance. Sync allows up to 61 minutes,
restart up to 6 minutes, and status/diagnostics up to 8 seconds. Command output
is capped at 64 KiB per command. Restart temporarily interrupts filesystem
access while the service flushes and remounts.

Operation progress lives in the plugin process and is lost when it stops.
After a plugin or server restart, inspect mount status before retrying an
operation: a container-side command may still be finishing. Use the normal
application Start/Stop controls for the installed application's lifecycle.

The host needs its normal LXD CLI access and the Go toolchain required by all
source plugins. S3disk continues to expose no container HTTP port.

## One-folder image

Everything specific to this mount extension lives here:

```
images/s3disk/
  image.json
  install.sh
  plugin/              host-side Go API and tests, compiled by Remote
  ui/                  mount controls
  container/           container-side Go module (cmd/, internal/, go.mod, go.sum, VERSION)
  container.tar.gz     generated payload — see "Refreshing the payload"
  package.sh           reproducible builder for the payload and for a .zip
  README.md
```

## What this package asks the host for

Nothing beyond the generic image contract. Remote reads `image.json`
generically and holds no knowledge of S3disk: the `env` block names the values
an operator supplies, `install.sh` provisions the mount inside the project's
container, and `plugin/` is compiled and run on the host to report status and
flush pending writes.

`install.sh` additionally declares the mount daemon to Remote's idle-workspace
probe by writing `/etc/remote/workspace-idle.d/s3disk`. The probe treats any
unrecognised process as active work, so without that declaration this
always-running daemon would pin every workspace it is installed in and no idle
project would ever be archived.

`container/` is a separate Go module because the mount has AWS/FUSE dependencies
and runs inside the container. The host plugin uses Remote's existing SDK.
The two execution locations do not require separate distributable packages.

## Refreshing the payload

That nested module is the whole reason `container.tar.gz` exists. `go:embed`
does not traverse a directory carrying its own `go.mod`, and it reports nothing
when it skips one — an unpacked `container/` would simply be absent from the
server binary, with a green build. `package.sh` packs it as a reproducible
archive instead — same source in, same bytes out, so rebuilding without having
changed anything leaves the tree clean:

```sh
./package.sh   # after editing anything under container/
```

Nothing else would notice the archive going stale, so
`TestS3diskPayloadMatchesContainerSource` in the parent package compares what
the archive holds against `container/` on disk. It runs in the `go test ./...`
CI already does, and names this script when it fails. The two agree on which
files belong in the payload, which means a change to that set belongs in both.

The script needs GNU tar: `--mtime`, `--owner`/`--group` and `--mode` pin every
field tar would otherwise read off the filesystem, and member order comes from
`sort`. bsdtar spells those differently or not at all, so the script checks and
exits rather than writing an archive that differs from machine to machine.

Remote's generic catalog loader base64-encodes the archive into the install
script, which unpacks it into a temporary directory inside the target container
and exports that as `APP_PACKAGE_DIR`; the staged source is removed when the
script exits. Installed binaries are stamped with VERSION plus a source hash,
so a source edit triggers a rebuild even when VERSION has not changed.

System packages (FUSE, certificates), a Go toolchain, and the third-party
dependencies pinned in `container/go.mod` are still fetched during installation.
The image is not an offline bundle of them.

## Building an uploadable `.zip`

Being built in is where this image lives, not what it is: the same folder is a
valid **uploaded package** (see
[16 — Uploaded packages](../docs/16-uploaded-packages.md)), which an
administrator adds to a running server from *Settings → Applications* without
waiting for a Remote release.

```sh
./package.sh --zip ~/s3disk.zip   # refreshes the payload, then archives it
```

The archive holds exactly what this directory holds, `container.tar.gz`
included — the server extracts a package as it arrives and never builds a
payload itself, so an image whose source is a nested module must ship the
archive that carries it. `container/` rides along beside it because `install.sh`
falls back to the source next to it when it was not staged from a payload.

The output goes outside the image on purpose, and the script refuses a path
inside it: `//go:embed images` would otherwise carry a copy of this folder,
inside this folder, into the server binary.

Two things to know before uploading one:

- **A package may not shadow a built-in image** (`registry.go`). On a Remote
  that already ships s3disk the upload is refused with `409`, since an upload
  may add an application rather than redefine one. This zip is for servers
  older than the release that built s3disk in, and for forks that give
  `image.json` an id of their own.

  A package uploaded *before* that release is not refused — it is already on
  disk. It is shadowed instead: the built-in image is served, the stored files
  become inert, and the package list says so and offers to remove them.
  Nothing has to be uninstalled to do that, because the copies installed under
  the id are the built-in image's from then on. Version `0.3.0` is above the
  `0.2.0` that was distributed as a zip so that those copies re-provision from
  the built-in source the next time they start.
- The container side is re-provisioned only when `version` moves. Uploading a
  changed `container/` under the version an installed copy already recorded
  leaves that copy on its old binary — see
  [17 — Versions and upgrades](../docs/17-versions-and-upgrades.md).

## Tests

`container/` is invisible to the `go vet ./...`, `go test ./...` and
`go build ./...` that CI runs from `backend/`: those skip nested modules
entirely. CI gives it a step of its own, and locally it is
`cd container && go test -race ./...`.

The host `plugin/` carries no module of its own — the server generates one that
pins the SDK — so it sits inside the backend module and is vetted, built and
tested along with every other package there.

## Moving chat attachments onto the bucket

Remote writes a chat attachment from the host into the directory the container
sees as `/workspace`, so it lands in `/workspace/.uploads`, beside the mount
and never inside it. A host-side write could not reach the mount even if it
aimed at one: the FUSE mount exists in the container's mount namespace, and
writing to the path underneath it would leave a file the mount then hides.

`POST push` closes that gap from the other side. It runs **inside the
container**, so the write goes through FUSE and s3disk uploads it. Give it
names alone:

```json
{"names": ["Screenshot-a8hocsqh.png"]}
```

It copies the file onto the mount, then deletes the copy in `.uploads`, and
reports each file as `stored`, `removed`, `skipped` (already on the bucket) or
failed, with the `path` it now lives at.

The order is the whole safety argument, and it only holds because the mount
writes back synchronously: `cp` returning means the object is in the bucket,
so deleting the other copy is redundancy removal rather than a bet. Nothing is
deleted when the copy failed. Nothing is deleted when the mountpoint is not
mounted — such a copy would land in the plain directory the mount hides, look
stored, and never reach S3. And nothing is deleted when the mount was given
`--async-writeback`, because a closed file may then still be only in the local
cache; those files are reported `stored` but kept, and **Flush mount writes**
followed by another push is the way through.

`S3DISK_UPLOADS_DIR` chooses where inside the bucket they land, defaulting to
`uploads`; `.` means its root.

### What calls it

`ui/scripts/uploadSync.js` subscribes to the SPA's `upload.completed`
extension event and **claims** each attachment, which makes the composer wait
before sending. That wait is not incidental: the prompt names the path it
hands the agent, and deleting the file from `.uploads` without re-pointing the
prompt would hand the agent a path to nothing. Once the plugin reports the
file removed, the claim resolves with its path on the mount and the prompt
points there instead. If the move fails at any step, the claim resolves with
nothing and the prompt keeps pointing at `.uploads`, which still holds the
file.

Attachments only ever go to the bucket of the project they were uploaded in.

It needs a Remote that emits `upload.completed`. An older one has no `events`
on the extension API at all, and an image manifest has no way to say which
Remote it needs, so the extension checks: it logs that attachments will stay
in `.uploads` and loads the mount controls anyway, rather than failing to
activate and taking the whole panel with it.

This path is browser-driven, and its limits follow from that:

- An upload written by an agent, the CLI or a terminal is not covered. Nothing
  emits the event for those, so they stay in `.uploads`.
- Closing the tab mid-move can leave an attachment copied but not deleted. It
  is on the bucket; the next push removes the local copy.
- `remote.backend.instances` is read when the extension loads, so installing
  s3disk into a project needs a page reload before that project's uploads
  start moving.

Nothing retries. **Attachment copies** in the mount controls lists what this
tab actually did, since the move is otherwise invisible.

## Flushing the mount

The **Flush mount writes** action calls this plugin's Go `sync` route, which
runs `s3disk sync` against the mountpoint and uploads dirty files from the
mounted bucket. It returns 202 immediately; poll `operation` for completion.

Stopping or uninstalling the plugin removes its UI contributions.

