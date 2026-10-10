# Project application web routes

## Reaching the application's HTTP server

Declare `"web": { "port": 8400, "subdomain": "editor" }`, a scope that includes `project`, and a service
listening on `0.0.0.0:8400`. A signed-in project member opens:

```text
https://remote.example/apps/my-project/editor/src/main.ts?line=12
    -> 302 https://editor--my-project.remote.example/src/main.ts?line=12
    -> proxy http://my-project.lxd:8400/src/main.ts?line=12
```

The launch URL uses the catalog application ID; the app hostname uses the
required manifest `web.subdomain` and project slug. Each application/project
pair has its own browser origin. Reinstalling preserves the origin. The main Remote origin only serves the launch redirect.

Every app-host request validates the session, registered account, caller's
project visibility, current catalog web declaration and running installation.
A URL cannot choose an arbitrary upstream host or port. `/api`, `/auth` and
`/internal` on an app hostname stay within that application's upstream and
never reach Remote's platform router. Unknown app hosts return 404.

The proxy preserves the request method, public Host, escaped path and query.
Relative assets and redirects work from `/`; applications need no URL prefix
and no Caddy rules. WebSocket upgrades use the same origin and access checks.
Remote does not rewrite HTML, JavaScript or application redirects.

All request cookies and `Authorization` are removed before forwarding.
Upstream `Set-Cookie` and `Clear-Site-Data` are removed, and responses use
`Cache-Control: private, no-store`. Apps that rely on their own browser cookies
are not supported by this gateway. Service workers are confined to the app's
origin; they cannot control Remote or another installation.

`web.port` must be 1024–65535, with a declared service and `project` in
`scopes`. `global` may be listed too, but only when
`globallyInstalledInsideContainers` is `true`: a global install then lands as a
project copy in every project, and each copy is routed like any other project
installation. An application whose global install uses a dedicated container
cannot declare `web`. No host port is allocated. Without `port.internal`, `APP_INTERNAL_PORT`
is zero and `healthcheck.command` must be omitted. Requests do not start
stopped installations or recreate containers. Missing/stopped installations
return 404; unavailable upstreams return 502. Upstream keep-alives are disabled.
See [12 — HTTP API](12-http-api.md#project-application-web-routes) for all statuses.

## Infrastructure

For application web routes, point `*.<public-host>` to the same server as Remote. An existing broader
DNS wildcard may already cover it; verify an installation hostname resolves.
The installer/updater installs one generic Caddy site block for this namespace.
There is no per-application Caddy configuration to maintain.

Caddy requests individual certificates on demand. The loopback TLS admission
endpoint approves only canonical manifest-label/project hosts belonging to a
running project web application whose project still exists. The wildcard DNS record is needed;
a wildcard certificate or DNS-provider API integration is not.

## Security boundary

The platform's domain-scoped HttpOnly session reaches the trusted Remote
gateway, which strips it before the container. Separate origins prevent app
JavaScript from reading Remote's DOM/storage. Browser Origin and Fetch Metadata
checks also reject cross-origin API requests, forms and WebSocket handshakes,
including same-site requests from sibling subdomains. Cookie stripping alone
would not provide that protection. Main-origin frame and opener restrictions
prevent app pages from embedding or retaining a privileged Remote window.

This does not sandbox admitted UI extensions, host backends or container
programs. Continue to install trusted packages. Read
[13 — Security model](13-security-model.md#project-application-web-content)
for the browser requirements and remaining OS/network trust boundaries.

All catalog-declared `web.port` values are excluded from public preview
sharing, even where the app is not installed. Changes to the current catalog
affect new and existing grants. This is a catalog-wide port reservation, not a
firewall or a restriction on authenticated previews or the shared LXD bridge.

## Source and verification

- [web.go](../../../backend/internal/service/applications/web.go) resolves running web installations without returning secrets.
- [application_host.go](../../../backend/internal/transport/http/application_host.go) owns installation hostname parsing.
- [applications_web_handler.go](../../../backend/internal/transport/http/handlers/applications_web_handler.go) owns host dispatch, launch redirects and caller/project authorization.
- [applications_web_proxy.go](../../../backend/internal/transport/http/handlers/applications_web_proxy.go) forwards to containers and filters credentials/response headers.
- [browser.go](../../../backend/internal/transport/http/middleware/browser.go) enforces browser request origins.
- [Caddyfile.tmpl](../../../infra/templates/Caddyfile.tmpl) routes application hostnames to the gateway.

Go tests cover launch redirects, escaped paths/queries, access denial,
stopped/missing apps, host isolation, credential stripping, TLS admission,
browser request policy and live WebSocket proxying. The opt-in Chromium test
checks authenticated launch, own-app requests, cookie filtering and rejected
platform reads, writes, logout, navigation and WebSockets. Run it as described
in [11 — Testing](11-testing.md#web-capability-checks).

These tests use local fixture services. Verify a real app's install/start,
assets, redirects and WebSockets on an LXD host, along with wildcard DNS and
on-demand TLS, when integrating a new application.

## Named application subdomains

Declare `web.subdomain` to use `<label>--<project-slug>.<public-host>`, for example
`code--gamerhead.remote.example.com`. The label comes from the manifest; the
project slug is its URL-safe name, not its display name or container ID.
The gateway uses the existing project membership check, then finds the running
application with that label in that project. Wrong labels, unknown projects, stopped installations
and duplicate running labels in one project are rejected. Web routes without `web.subdomain` and previous installation-ID hosts are rejected.

Deploy the updated Caddy template for these single-label names. Its wildcard site
forwards application hosts to the existing backend gateway;
the gateway reserves application origins from platform API/login routing and
returns 404 for unknown applications. Existing preview and built-in code namespaces
retain their specific Caddy routes. The combined app/project label must fit 63 characters. Project names and manifest
labels cannot contain the reserved `--` separator. Single hyphens are allowed. DNS
must resolve the full hostname to this ingress.

Reinstalling an app preserves its hostname and browser storage. Changing the
manifest label or project slug changes its origin, requiring updated bookmarks
and PWA installs. Server-side Code Server settings remain separately persisted.

The named host occupies one DNS label below the full platform hostname. It gets
its own on-demand certificate, admitted like a preview; no DNS provider is
required. See [Code Server wildcard HTTPS](../wildcard-https.md). Preview TLS behavior remains unchanged.
Existing projects are not renamed automatically. Any legacy slug containing `--`
cannot be used for a named application host.
