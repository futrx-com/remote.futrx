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
| Container service | `backend/container/cmd/file-manager/` builds `/usr/local/bin/file-manager` |
| Lifecycle | `application.json` declares `remote-file-manager`, listening on port 8844 |
| Browser surface | `ui/scripts/main.js` registers the Files drawer |
| Files page | `backend/container/internal/presentation/web/assets/`, embedded in the service binary |
| Cleanup | `infra/uninstall.sh` removes the binary after Remote stops the service |

There is no host backend, custom install script, database, configuration input,
or agent skill. Remote builds the container program and manages the service.
Its authenticated web gateway exposes the service at the project's `files`
application hostname. No independent login or public host port is configured.
As with Terminal, the service relies on the gateway for user authentication and
project membership; its container listener is not an independent auth boundary.

## Container application layers

`backend/container/` is an independent Go application. Its generated module
contains only container source; it does not import the host backend or Remote's
internal packages. The binary name remains `file-manager`.

```text
backend/container/
  cmd/file-manager/main.go         command and dependency composition
  internal/
    config/                       workspace, server settings, resource limits
    model/files/application/      application results and errors
    port/files/inbound/           file operations exposed to HTTP handlers
    port/files/outbound/          filesystem and archive-spool contracts
    service/files/                listing/search policy and download workflows
    repository/workspace/         rooted filesystem, listing, search, ZIP I/O
    repository/archives/          bounded temporary files and admission slots
    transport/http/               server, mux and response serialization
    transport/http/handlers/      RegisterRoutes, file handlers and error mapping
    transport/http/middleware/    response security headers
    presentation/web/assets/      embedded browser page
```

The command injects concrete repositories into the file service, then injects
that service into HTTP handlers. The transport composes their `RegisterRoutes`
methods with the static page and middleware, matching Remote's own HTTP layout.
Services do not depend on HTTP or concrete filesystem repositories. Handlers
map application results into JSON
and streams completed downloads. Archive preparation, spooling, and failure
cleanup run behind the service contract. Each spool instance owns its limits
and concurrency slots; request handlers do not mutate package globals.

The embedded page has its own frontend layers under
`internal/presentation/web/assets/`:

```text
assets/
  main.js                              dependency composition
  api/filesApi.js                      HTTP calls and download URLs
  controllers/filesPageController.js    search events and drawer-message handling
  services/fileBrowserService.js        tree/search state and request cancellation
  presentation/fileBrowserView.js       DOM rendering and theme application
  transport/parentBridge.js             parent-window messages and origin checks
  config/files.js                      icons, file categories and search timing
  index.html
  style.css
```

The controller delegates user intent to the service and renders its state
through the view. The service receives its API dependency and has no DOM
dependency; the view has no network access. The parent bridge owns the iframe
protocol, and `main.js` supplies the concrete dependencies. The two frontend
surfaces use the same layer vocabulary but remain independently served.

A host API application, when needed, is a separate composition at
`backend/main.go` with its own host packages, as in Code Server. Remote excludes
`backend/container/` from that host build. File Manager's existing behavior uses
its container web service directly and needs no host RPC process.

## Use

Open Files in the chat header. Expand folders, refresh the tree, or search
filenames with at least two characters. Dotfiles are included. Downloads support
individual files and folder ZIPs, including the workspace root through
`/api/archive`. Listings stop at 10,000 entries; searches stop at 300 matches or
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
The iframe bridge checks both the sender window and exact application origin;
project and chat context come from the drawer, not from iframe messages.

The drawer keeps the look of Remote's former built-in Files pane. Remote draws
the header; the drawer sets its subtitle (`workspace`, `Loading...`, or the
result count) and Refresh button from the page's `state` messages, and forwards
Refresh and theme tokens back to the page.

## Remote extension layers

The extension runs in Remote's page; the container-served files page above is
a separate browser surface. The extension uses these layers:

```text
ui/
  scripts/
    main.js                         dependency composition and registration
    controllers/
      filesDrawerController.js      drawer lifecycle, header and iframe messages
    services/
      fileOpeningService.js         core/fallback workflow and cached availability
      fileAction.js                 extension-based media/editor/download policy
    api/
      remoteFilesApi.js             optional Remote file-opening API
      projectApplicationsApi.js     running Code Server check over HTTP
      applicationUrls.js            project application origins and drawer URL
      fileUrls.js                   editor, Scratch Tab, media and download URLs
    transport/
      browserNavigation.js          link dispatch, new tabs and browser downloads
    presentation/
      drawerTheme.js                theme subscriptions and cleanup
    config/
      drawer.js                     header icons, theme tokens and breakpoint
      files.js                      Code Server identity and file-type catalogs
  tests/                            unit and browser checks
```

`main.js` constructs the concrete dependencies and supplies them to the
controller and service. The controller validates iframe messages and delegates
file clicks to the service. The service depends only on injected capabilities
and the file-type policy; it does not import API or transport implementations
or access `window`, `document` or `fetch`. It retains the original synchronous
opening sequence, cached availability, and status refresh after each click.

All file operations stay inside `/workspace`, including symlink resolution and
rooted final opens. ZIPs skip special files and escaping symlinks, honor request
cancellation, and bound source bytes, entry count, compressed spool size (1 GiB),
and concurrency (two archives). Temporary archives are deleted after download.

## Lifecycle and verification

Stop removes the drawer and stops the service; start restores it. Uninstall
removes the application binary and Remote-owned service files, preserving all
workspace files. Reinstall builds from the current package. When changing the
application, bump its manifest version so installed copies reconverge.

Run from the repository root (the optional browser check needs Node, Playwright,
and its Chromium installation):

```bash
go test ./applications/file-manager/backend/container/...
go vet ./applications/file-manager/backend/container/...
node --test applications/file-manager/ui/tests/*.test.mjs
FUTRX_BROWSER_TEST=1 go test -count=1 ./applications/file-manager/backend/container/... -run TestFileManagerBrowser -v
```

After a real project installation, check the Files drawer, search, nested folder
ZIPs, media previews, and Code Server opening. Confirm another project has no
Files action until installed. Then install globally and confirm each existing
project gets its own copy and browses only its own workspace. Create another
project and repeat the global install to add its copy, preserving existing ones.
Check stop/start/uninstall per project and container recovery.
Inside the target container, verify `systemctl status remote-file-manager` and
`curl -fsS http://127.0.0.1:8844/health`. Container provisioning and authenticated
web-gateway routing need a live Remote/LXD installation; unit tests do not prove
those paths.
