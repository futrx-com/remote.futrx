# Code Server

Optional browser editor for a project workspace. Install it from that project's
Applications page. Its icon appears in the chat header only while the project
installation is running. The editor opens at
`https://<host>/<project-slug>/code/`. The existing `code.<host>/<slug>/`
route and PWA launcher continue to open the same workspace.

Before selecting **Install**, edit the complete **VS Code settings.json** field
in the catalog's install form. It starts with the current Remote defaults from
`infra/settings.json` and accepts any valid JSON object of VS Code settings.
The selected document is validated and stored with the project application.
Once installed and running, use the **Settings** action on Code Server's
installed row to edit it again. That form calls this application's Go backend,
which validates the document and writes it to Code Server's active settings.
The editor is available through Remote even if nobody opens Code Server itself.

The backend also writes a private durable copy to
`/workspace/.remote/code-server/settings.json`. The install script uses that
copy when a project container is replaced or the application is upgraded. A
new installation replaces an older uninstalled copy with its own install-form
settings when its backend starts. The install-form value is treated as a secret
because extension settings may contain credentials, and both container files
are written with mode `0600`. A custom `window.title` is retained; the default
placeholder becomes the container hostname. Code Server's bind address,
authentication, and socket ports remain owned by Remote so its IDE route
continues to work.

The install downloads Code Server 4.121.0 for amd64 or arm64, applies the
current workspace settings and extensions, and creates a socket-activated
service. The listening socket uses little memory; the editor process starts
when someone opens it and stops after the proxy has been idle for ten minutes.
Stop or uninstall disables the socket and stops both processes. Uninstall leaves
the package, settings, and extensions in the project container; it can be
installed again without redownloading the matching package. Deleting or
recycling the project container removes those files; the saved install settings
are reapplied when a running installation is restored.

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
- Settings edited directly inside Code Server change its active file but do
  not update the durable copy. Use the application's form for changes that
  must survive a container replacement.
- The durable copy sits in the project workspace. Project users with file
  access can read it, as they can read Code Server's active settings inside
  the project container. Do not put credentials there unless that access is
  acceptable.

Verification in a running project container: `systemctl is-active
code-server.socket` reports `active`, while `code-server.service` may be
inactive until the first visit. Open the project IDE URL through Remote's
authenticated Caddy route, then check `code-server.service` and
`code-server-proxy.service`. Stop the application and confirm the icon is gone
and the socket is inactive. The project port 8842 is reached directly from
Caddy on the LXD bridge; no public host port is allocated.

This application is project scoped because the current IDE runs in the project
container and edits `/workspace`. Global installation would run in a separate
container without that workspace. On older project containers, Code Server may
already be installed by the old base image; the application takes over its
systemd units when installed.
