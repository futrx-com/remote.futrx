# Code Server

Optional browser editor for a project workspace. Install it from that
project's Applications page. Its icon appears in the chat header only while
the project installation is running. Editor and file links open directly at
`https://<web.subdomain>--<project-slug>.<host>/`. The editor, assets and WebSockets stay on
that project's application origin. Configure DNS for `<web.subdomain>--<project-slug>.<host>` to reach Remote;
The shared gateway infrastructure uses one certificate for the platform hostname
and one wildcard certificate for named application hosts, with an administrator-selected
DNS provider. Code Server does not request a certificate for each project. Existing
preview URLs and their certificate handling remain unchanged.

The manifest declares `"web": { "port": 8842, "subdomain": "code" }`. The label
comes from `remote.application.web.subdomain`; the project slug comes from the
selected workspace path. Neither a container ID nor an application installation ID
is used in the URL. For project `gamerhead` on `remote.example.com`, the URL is
`https://code--gamerhead.remote.example.com/?folder=%2Fworkspace`. Reinstalling
preserves the URL. The existing gateway resolves it to the current running install.
DNS must resolve that full hostname to Remote, and Caddy must use the updated
wildcard application-host route from the shared prerequisite.

The UI extension selects the running installation for the current project from
`remote.backend.instances`. File-opener callbacks receive `projectId` from core.
The host refreshes installation metadata on reconciliation, so subsequent links
reflect current availability while keeping the same origin after a reinstall.

File links open `/_static/remote-open.html` on the application subdomain. This
application-owned page builds Code Server's `openFile` payload on that origin,
preserving the folder, file, line and column.

## Install as a desktop or mobile app

Open the project editor URL in a browser, then use the browser's **Install app**
action (or **Add to Home Screen** on iOS). Code Server provides the PWA
manifest, icons, and service worker. Its installed name includes the project
container's hostname so editors from different projects are distinguishable.
Each project application has its own origin and service worker, separate from Remote
and other projects. Reinstalling keeps the same project URL and browser storage, so existing PWA
bookmarks continue to work once the application is running again.
The editor still needs a network connection and a running Code Server
installation. Stopping or uninstalling Code Server makes the installed app's
launch URL unavailable while stopped; the origin becomes available again after reinstall.

Before selecting **Install**, edit the complete **VS Code settings.json** field
in the catalog's install form. It starts with the current Remote defaults from
`infra/settings.json` and accepts any valid JSON object of VS Code settings.
The selected document is validated and stored with the project application.
After selecting **Install**, closing the dialog or navigating away does not
cancel provisioning. Return to Applications to see its running or failed state.
Once installed and running, use the **Settings** action on Code Server's
installed row to edit it again. That form calls this application's Go backend,
which validates the document and writes it to Code Server's active settings.
The editor is available through Remote even if nobody opens Code Server itself.

Code Server's User directory is stored at
`/workspace/.remote/code-server/User` and linked from its normal location.
Settings edited in Code Server and settings saved through Remote use the same
persistent files, including when the editor saves by atomic file replacement.
Updates and container replacement retain settings, keybindings, profiles and
other User-directory state. During migration, existing editor files take
precedence over the older saved settings copy. If only that copy survives,
it is restored. The install form seeds settings only when no existing settings
are available; backend startup never resets them to the install form.
The persistent directory is private (`0700`) and settings use `0600`.
A custom `window.title` is retained; the default
placeholder becomes the container hostname. Code Server's bind address,
authentication, and listening port remain owned by the application so its IDE route
continues to work.

The install form exposes **Code Server version**, defaulting to `4.121.0` in
`application.json`. Enter a release as `major.minor.patch`. Installation downloads
that version for amd64 or arm64, applies the
current workspace settings and extensions, and starts a regular systemd
service listening on `0.0.0.0:8842`. The editor stays running while the
application is enabled; closing browser tabs does not stop it. Stop stops the
service, keeping the package and settings for a later start. Uninstall also
purges the Code Server package and
removes its configuration, extensions, caches, and saved settings, including
`/workspace/.remote/code-server`. Reinstalling downloads the package and uses
the new install form settings. A running installation restored into a new
project container is provisioned again using its persistent User directory.

The Files drawer downloads source files when Code Server is not running. A
direct IDE link still requires the app and returns 404 while it is stopped or
uninstalled. A workspace upgrade re-installs running project applications in
the replacement container; stopped installations stay stopped. Normal project
startup also restores running applications after a missing container is
recreated. Starting a stopped installation after container replacement
reinstalls its missing service and package.

Settings editing has these boundaries:

- Before installation, the backend does not exist, so the catalog's existing
  JSON install form supplies the initial document. The application-owned form
  appears only after installation.
- The application backend is unavailable while the app is stopped. Start the
  app to edit settings; the saved document stays in place while stopped.
- The form accepts strict JSON objects up to 128 KiB. JSON with comments or
  trailing commas is not accepted. It controls Code Server's user
  `settings.json`, not Remote-managed `config.yaml` transport settings.
- Direct editor changes persist immediately. Existing JSON with comments or
  trailing commas is preserved byte-for-byte by installation and migration;
  Remote's form still requires strict JSON to read or save that document.
- The durable copy sits in the project workspace. Project users with file
  access can read it, as they can read Code Server's active settings inside
  the project container. Do not put credentials there unless that access is
  acceptable.

Verification in a running project container: `systemctl is-active
code-server.service` reports `active` after installation, before any browser
visit. Open the project IDE URL through Remote's authenticated application
web route. Close the tab and confirm the service stays active. Stop the
application and confirm the icon is gone and the service is inactive.
Remote's authenticated application web proxy
reaches project port 8842 on the LXD bridge; no public host port is allocated.

This application is project scoped because the current IDE runs in the project
container and edits `/workspace`. Global installation would run in a separate
container without that workspace. On older project containers, Code Server may
already be installed by the old base image; the application takes over its
service when installed and removes the legacy socket/proxy units.

## Security boundary

The web proxy checks project membership and a running installation and strips
cookies and Authorization before forwarding. The editor runs on an isolated
installation origin. Remote's browser protections reject cross-origin platform
API and WebSocket requests, including requests from sibling app subdomains.
The trusted UI extension and settings backend remain privileged application
code. The LXD bridge still permits direct connections from sibling containers
to the editor socket; origin isolation does not provide container network isolation.

Settings and launch regression checks: `node --test applications/code-server/infra/*.test.mjs`.
The application uses a regular service without idle shutdown and retains the
subdomain-aware file-launch page without resetting settings.
In a disposable project, change a setting directly in Code Server, upgrade the
app, then replace the container and confirm the setting and keybindings remain.

### Backend ownership

- [main.go](backend/main.go) wires the settings service to container I/O and starts
  RPC through `ServeWithRuntime`, matching Hello Remote. Code Server currently
  declares no publishers or subscriptions and does not use runtime events.
- [api/api.go](backend/api/api.go) owns routes, request serialization, and HTTP error mapping.
- [settings/store.go](backend/settings/store.go) owns instance initialization and settings validation. Initialization never writes settings.
- [containerio/settings.go](backend/containerio/settings.go) owns timed LXC commands; [write-settings.js](backend/containerio/write-settings.js) performs the existing atomic save and is embedded into the backend.
- [config/settings.go](backend/config/settings.go) holds the size limit, timeout, and active settings path.

The installer invokes `infra/migrate-settings.cjs` to seed and migrate the
persistent User directory. The infrastructure tests execute that script and
the embedded settings writer.

The selected Code Server version is saved with the installation. Application
updates preserve that choice; changing the manifest default affects new installs.

### Installer assets

The file-launch page lives in `infra/remote-open.html`; settings migration lives
in `infra/migrate-settings.cjs`. Remote stages both through its existing
`infra/payload.tar.gz` support. `install.sh` copies the page and runs the migration.
After changing an asset, run `bash applications/code-server/infra/build-payload.sh`
from the repository checkout. Payload tests compare the shipped archive with
its reviewable sources so an outdated archive fails validation.

Editor URL parsing and validation live in `ui/scripts/editorUrls.js`;
`ui/scripts/main.js` registers the editor actions and file opener.
