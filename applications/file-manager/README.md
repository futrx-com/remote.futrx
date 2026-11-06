# File Manager

Optional workspace browsing. Install **File Manager** from a project's
Applications tab, or install it globally from **Settings → Applications** to add
it to every existing project. Global installs use each project's container
because `globallyInstalledInsideContainers` defaults to `true` when omitted from
the manifest. The `scopes` list still explicitly enables both `project` and
`global` installation. Each copy is managed from its project's Applications tab,
and its Files drawer appears beside the chat
only while that copy is running. New projects do not inherit it; run the global
install again to add missing copies without replacing existing installations.

## Package

| Capability | Owner |
|---|---|
| Host backend | `backend/` registers the `list`, `search` and `archive` routes |
| Container command | `backend/container/cmd/file-manager/` builds `/usr/local/bin/file-manager`, run once per request |
| Browser surface | `ui/scripts/main.js` registers the Files drawer and renders it in Remote's page |
| Upgrade | `infra/install.sh` retires the container web service from versions before 5 |
| Cleanup | `infra/uninstall.sh` removes the binary and any retired service |

There is no container web service, port, application hostname, database,
configuration input, or agent skill. The drawer calls its project
installation's backend through `remote.backend`, at
`/api/projects/<project>/applications/<instance>/backend/<route>`, where Remote
checks the session and project membership on every request. Each route runs
`lxc exec <container> -- /usr/local/bin/file-manager <command>` with the
request as JSON on stdin, so browser input never becomes a program argument.
Single files download through Remote's own
`/api/chats/<chat>/files/download` route, which streams them; only folder ZIPs
are built by the backend.

## Host backend layers

```text
backend/
  main.go           composition: the container runner into the API
  api/              routes, input bounds, response and status mapping
  service/archives/ archive admission and the shared waiting/build time budget
  containerio/      the fixed `lxc exec` command, exit statuses, output limits
  config/           timeouts and limits
```

The API re-encodes listings through typed values rather than passing the
container's output through and names ZIP downloads itself. The archive service
admits two builds at a time per installation; waiting for admission and building
share one time budget. `containerio` maps the container
command's exit statuses (3 invalid path, 4 not found, 5 too large) to errors
and refuses output past each route's limit.

## Container application layers

`backend/container/` is an independent Go application. Its generated module
contains only container source; it does not import the host backend or Remote's
internal packages. The binary name remains `file-manager`.

```text
backend/container/
  cmd/file-manager/main.go         command and dependency composition
  internal/
    config/                       workspace and resource limits
    model/files/application/      application results and errors
    port/files/inbound/           file operations exposed to the CLI
    port/files/outbound/          filesystem and archive-spool contracts
    service/files/                listing/search policy and archive workflow
    repository/workspace/         rooted filesystem, listing, search, ZIP I/O
    repository/archives/          bounded temporary files
    transport/cli/                stdin request, stdout result, exit statuses
```

The command injects concrete repositories into the file service, then hands
that service to the CLI transport. Services do not depend on the transport or
concrete filesystem repositories. An archive is completed in a bounded spool
before anything is written to stdout, so a failed build never looks like the
start of a ZIP.

## Remote extension layers

```text
ui/
  scripts/
    main.js                         dependency composition and registration
    controllers/
      filesDrawerController.js      drawer lifecycle, header, search input, opens
    services/
      fileBrowserService.js         tree/search requests and cancellation
      rootPrefetchService.js        throttled, one-use workspace root prefetch
      fileOpeningService.js         core/fallback workflow and cached availability
      fileAction.js                 extension-based media/editor/download policy
    api/
      filesApi.js                   backend routes and download URLs
      remoteFilesApi.js             optional Remote file-opening API
      projectApplicationsApi.js     running Code Server check over HTTP
      applicationUrls.js            project application origins
      fileUrls.js                   editor, Scratch Tab, media and download URLs
    transport/
      browserNavigation.js          link dispatch, new tabs and browser downloads
    presentation/
      fileBrowserView.js            drawer layout and DOM rendering
      paneSettle.js                 delay first render until opening has settled
    state/
      fileBrowserState.js           tree/search data, transitions and notifications
    config/
      drawer.js                     header icons
      files.js                      icons, file categories, search timing
  style/files.css                   styles scoped to the drawer
  tests/                            unit and browser checks
```

`main.js` constructs the concrete dependencies and supplies them to the
controller. The browser service receives its API and state owner and has no DOM
dependency; the view has no network access. Prefetch reuse is composed around
the backend API, which handles only requests and download URLs. The file-opening service depends only on injected
capabilities and the file-type policy; it retains the original synchronous
opening sequence, cached availability, and status refresh after each click.
The drawer renders in Remote's page, so it inherits Remote's theme tokens
directly and every style rule is scoped under `.remote-files`.

## Use

Open Files in the chat header. Expand folders, refresh the tree, or search
filenames with at least two characters. Dotfiles are included. Downloads support
individual files and folder ZIPs, including the workspace root through the
`archive` route. Listings stop at 10,000 entries; searches stop at 300 matches or
200,000 visited entries and report truncation.

File clicks use the shared `remote.files.open` API: supported media opens in
Remote's media viewer, source/text files use an installed application opener
(Scratch Tab claims them inside Remote's page; otherwise Code Server opens in a
new tab), and other files download. Both editors are optional. If the browser
blocks the editor tab, the drawer shows a link to open it.

Remote versions without `remote.files.open` give applications no access to
other applications' openers. There, the file-opening service reproduces the
former built-in drawer from Remote's page: it offers the file as a link click
that Scratch Tab can claim, opens Code Server in a new tab when Remote's
`/api/projects/{id}/applications` reports it running, opens media in a new tab,
and downloads everything else. It mirrors Code Server's and Scratch Tab's link
formats, so it is a stopgap until Remote ships `remote.files.open`.
Project and chat context come from the drawer. A listed path that is absolute
or contains `.` or `..` segments never reaches an opener.

While a project chat shows the Files toggle, the extension fetches that
project's workspace root at most once every 15 seconds; the next drawer to open
shows it at once, and Refresh always fetches again. The tree is drawn only once
Remote's pane has finished opening, so the opening animation does not stall.
`backend.background` restores the backend after Remote restarts, so the first
open does not wait for it to start.

The drawer keeps the look of Remote's former built-in Files pane. Remote draws
the header; the drawer sets its subtitle (`workspace`, `Loading...`, or the
result count) and its Refresh button.

All file operations stay inside `/workspace`, including symlink resolution and
rooted final opens. ZIPs skip special files and escaping symlinks and bound
source bytes (1 GiB), entry count (200,000), compressed size (256 MiB) and
concurrency (two archives per installation). The backend holds a finished ZIP
in memory to answer its call, which is why the compressed limit is lower than
the source limit. The container's temporary archive is deleted once it has
been written out. A ZIP must finish within the backend's five-minute call
limit.

## Upgrading from version 4

Versions up to 4 ran `remote-file-manager.service` in the container and served
the page from `files--<project-slug>.<host>`. Version 5's `infra/install.sh`
disables and removes that unit during the upgrade, and `infra/uninstall.sh`
removes it from copies that were never upgraded.

## Lifecycle and verification

Stop removes the drawer and stops the backend; start restores both. Uninstall
removes the application binary, preserving all workspace files. Reinstall builds from the current package. When changing the
application, bump its manifest version so installed copies reconverge.

Run from the repository root (the optional browser check needs Node, Playwright,
and its Chromium installation):

```bash
go test ./applications/file-manager/...
go vet ./applications/file-manager/...
node --test applications/file-manager/ui/tests/*.test.mjs
FUTRX_BROWSER_TEST=1 node --test applications/file-manager/ui/tests/browser.test.mjs
```

After a real project installation, check the Files drawer, search, nested folder
ZIPs, media previews, and Code Server opening. Confirm another project has no
Files action until installed. Then install globally and confirm each existing
project gets its own copy and browses only its own workspace. Create another
project and repeat the global install to add its copy, preserving existing ones.
Check stop/start/uninstall per project and container recovery.
Inside the target container, `/usr/local/bin/file-manager list </dev/null`
prints the workspace root as JSON, and nothing listens on port 8844. Container
provisioning and `lxc exec` from the backend need a live Remote/LXD
installation; unit and browser tests do not prove those paths.
