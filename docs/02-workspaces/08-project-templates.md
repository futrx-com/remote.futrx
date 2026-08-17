# Project templates

A **template** (stack preset) decides what is installed inside a project's
container. It is chosen once, in the new-project dialog, and cannot be changed
afterwards.

Templates are a **layer**, not a fleet of images. The target deployment is a
single small server, so building one complete image per stack would be
expensive in build time and disk. A template is therefore:

1. the shared base image `futrx-remote-dev-base`, plus
2. a post-provision script run once inside the new container, plus
3. optional seed files written into the durable `/workspace` mount, plus
4. an optional agent-instructions snippet seeded as `/workspace/AGENTS.md`.

A template may *additionally* declare a dedicated pre-built image alias. When
that image is published on the host, the container launches straight from it
and the provisioning is already baked in.

```mermaid
flowchart TD
    Create["Create project with template T"] --> Resolve{"futrx-remote-T-base published?"}
    Resolve -- yes --> Fast["lxc init futrx-remote-T-base<br/>(fast path: stack already installed)"]
    Resolve -- no --> Slow["lxc init futrx-remote-dev-base<br/>(slow path)"]
    Fast --> Marker{"/var/lib/remote-template.done present?"}
    Slow --> Marker
    Marker -- yes --> Ready["Nothing to do"]
    Marker -- no --> Provision["Seed /workspace files, then run the provision script<br/>logging to /var/log/remote-template.log"]
    Provision --> Done["Write the marker; report done"]
```

## What each template installs

| Template | Installs | Ports | Pre-built image |
| --- | --- | --- | --- |
| `blank` (default) | Nothing beyond the base image: Ubuntu 24.04, Node 22, Python 3, git, gh, jq, the agent CLIs, code-server, Chromium | — | not applicable |
| `wordpress` | PHP 8.3 CLI + `mysql`, `curl`, `gd`, `mbstring`, `xml`, `zip`, `intl`, `bcmath`, `soap`; MariaDB (enabled as a boot service, root over the unix socket, no password, database `wordpress`); Composer; WP-CLI at `/usr/local/bin/wp` plus a `wp-root` wrapper; WordPress core downloaded to `/workspace/public` with a generated `wp-config.php`; `remote-wordpress.service` serving `/workspace/public` on port 8080 with PHP's built-in server | 8080 | `futrx-remote-wordpress-base` |
| `laravel` | PHP 8.3 CLI + `mysql`, `sqlite3`, `curl`, `mbstring`, `xml`, `zip`, `intl`, `bcmath`, `gd`; MariaDB (boot service, database `laravel`); Composer; a `laravel/laravel` skeleton in `/workspace/app` with `.env` pointed at the local database. Node 22 for Vite comes from the base image | 8000, 5173 | `futrx-remote-laravel-base` |
| `node` | `pnpm` and `yarn` enabled through corepack, plus a `/workspace/README.md`. Nothing is downloaded from apt | 3000, 5173 | none |
| `python` | `python3.12-venv`, `python3-dev`, `pipx`, `uv` at `/usr/local/bin/uv`, and a project virtualenv at `/workspace/.venv`, plus a `/workspace/README.md`. Poetry is deliberately not installed | 8000 | none |

WooCommerce is **not** preinstalled on the WordPress template. Install it in a
project when it is needed:

```bash
wp --allow-root --path=/workspace/public plugin install woocommerce --activate
```

The port column is informational. Previews are discovered by scanning the
container for listening sockets, so any port bound to `0.0.0.0` becomes a
preview URL regardless of what the template declares.

## Provisioning contract

Every template's provision script runs inside a shared harness
(`ProvisionProgram` in
[`backend/internal/service/container/templates/provisioning.go`](../../backend/internal/service/container/templates/provisioning.go)),
which supplies:

- `set -eu`, `DEBIAN_FRONTEND=noninteractive`, `NEEDRESTART_MODE=a`, and
  `APT_LISTCHANGES_FRONTEND=none`, so no install can prompt;
- an `apt_retry` helper that runs `apt-get`, then refreshes the index and
  retries **once** — the templates must use it instead of bare `apt-get`;
- output teed to `/var/log/remote-template.log` inside the container;
- the marker protocol below.

| Path (inside the container) | Meaning |
| --- | --- |
| `/var/log/remote-template.log` | Append-only log of every provisioning run |
| `/var/lib/remote-template.done` | Written only after the script completes. Its presence makes every later run a no-op |
| `/var/lib/remote-template.failed` | Written when a run starts, removed on success. Its presence after a backend restart is what distinguishes "failed" from "pending" |

All three live in the **disposable** container root filesystem, on purpose.
`upgrade-workspaces` re-clones every container from the image, which discards
them — and that is exactly the intent: a replaced container must be
re-provisioned. The lifecycle offers provisioning on **every** convergence
([`lifecycle/service.go`](../../backend/internal/service/container/lifecycle/service.go)),
and the marker file is what makes that offer cheap: one `test -e` per project
start.

Seeding never overwrites. `/workspace` is durable and may already hold the
user's own version of a path, so a seed whose target exists is skipped.

## Provisioning status

Provisioning runs in the background — a WordPress stack on a 1 vCPU host takes
minutes — so project creation returns immediately and the status is polled.

| Status | Meaning |
| --- | --- |
| `none` | The template installs nothing (`blank`); the container was ready on first boot |
| `pending` | Provisioning is required and has not started, or the backend restarted before it could |
| `running` | The provision script is executing |
| `done` | The marker file is present |
| `failed` | The last attempt exited non-zero |

The status is part of the project status payload:

```bash
curl -s -b cookies.txt https://<host>/api/projects/<id>/container | jq .template
```

```json
{
  "name": "wordpress",
  "title": "WordPress",
  "status": "running",
  "logPath": "/var/log/remote-template.log",
  "startedAt": 1755400000000
}
```

The project settings page shows the same thing as a badge next to the project
name and as a **Template** panel on the Info tab.

A failed run is retried on the next convergence, so **restarting the project**
retries provisioning. Read `/var/log/remote-template.log` inside the container
for the cause.

## Building a pre-built template image on a beefier server

The slow path costs every new project the full install. Publishing a dedicated
image moves that cost to build time, once per host:

```bash
cd /opt/remote.futrx/backend

# Which templates can be baked into an image
go run ./cmd/build-template-image -list

# Build. The builder starts from futrx-remote-dev-base, so the base image
# must already be published (infra/steps/05-base-image.sh does that).
go run ./cmd/build-template-image -template wordpress

# Replace an existing one
go run ./cmd/build-template-image -template wordpress -overwrite
```

Publishing compresses the whole root filesystem and regularly exceeds the
default 5-minute budget on a 1 vCPU box. Both budgets are overridable, by flag
or environment variable:

```bash
go run ./cmd/build-template-image -template wordpress \
  -build-timeout 60m -publish-timeout 30m

FUTRX_IMAGE_BUILD_TIMEOUT=60m FUTRX_IMAGE_PUBLISH_TIMEOUT=30m \
  go run ./cmd/build-template-image -template wordpress
```

The image is a plain LXD image, so it can be built on a bigger machine and
moved:

```bash
# On the build host
lxc image export futrx-remote-wordpress-base wordpress-base
scp wordpress-base.tar.gz root@server:/tmp/

# On the small server
lxc image import /tmp/wordpress-base.tar.gz --alias futrx-remote-wordpress-base
```

`GET /api/templates` reports `prebuiltImageAvailable` per template, and the
new-project dialog shows a **prebuilt image** tag on those cards, so you can
confirm the fast path is live without touching the host.

### Keeping template images current

`infra/upgrade-workspaces.sh` rebakes `futrx-remote-dev-base` and recycles
containers onto it. Projects whose template has a published image launch from
**that** image, not from the base, so the base rebake alone leaves them on the
old layer. The script detects published `futrx-remote-*-base` aliases and warns
about it; pass `--rebake-templates` to rebuild each of them on top of the fresh
base:

```bash
sudo bash /opt/remote.futrx/infra/upgrade-workspaces.sh --rebake-templates
```

The rest of the script's behaviour is unchanged: containers with an active
agent process are still skipped unless `--include-busy` is given, and
`/workspace` plus the provider homes still survive.

Deleting a template image is always safe. The next project on that template
falls back to base + provision script.

## Adding a custom template

Templates are data files embedded into the binary. Adding one is a code change,
not a runtime configuration:

1. Create `backend/internal/service/container/templates/<name>/` — the
   directory name **is** the template name and must match `[a-z][a-z0-9-]{0,31}`.
2. Write `template.json`:

   ```json
   {
     "name": "rails",
     "title": "Ruby on Rails",
     "description": "Shown on the picker card. One or two sentences.",
     "icon": "blank",
     "provisionScript": "provision.sh",
     "agentInstructions": "AGENTS.md",
     "seedFiles": [
       { "source": "README.md", "target": "/workspace/README.md", "mode": "644" }
     ],
     "defaultPorts": [3000],
     "prebuiltImage": true
   }
   ```

   Every field except `name`, `title`, `description`, and `icon` is optional.
   Unknown fields are rejected at load time. `seedFiles[].target` must be a
   clean absolute path under `/workspace/`. `prebuiltImage: true` only means
   the runtime will *look* for `futrx-remote-<name>-base`; it does not create
   it.

3. Write `provision.sh` as a **payload**, not a standalone script: no shebang,
   no `set -e`, no `DEBIAN_FRONTEND`, no marker handling — the harness owns all
   of those. Use `apt_retry` for apt calls. A test enforces this.

4. Write `AGENTS.md` describing the stack for agents: what is installed, where,
   which commands to use, and the preview rules. It is seeded to
   `/workspace/AGENTS.md` and never overwrites an existing file.

5. Add the directory to the `//go:embed` line in
   [`catalog.go`](../../backend/internal/service/container/templates/catalog.go)
   and give it a position in the `order` map (unlisted templates sort
   alphabetically after the listed ones).

6. Optionally map the `icon` key to a glyph in
   [`frontend/src/ui/projects/templateIcons.tsx`](../../frontend/src/ui/projects/templateIcons.tsx).
   An unmapped key falls back to a generic icon rather than failing.

7. Run `go test ./internal/service/container/templates/...`. Catalog
   validation, the harness contract, and the shipped-template properties are
   all covered there.

## Backward compatibility

Project metadata (`DATA_DIR/projects/<id>/meta.json`) gained a `template`
field. It is omitted when the project uses the default, so metadata for a
`blank` project is byte-compatible with what earlier releases wrote, and a
`meta.json` without the field reads as `blank`.

## Why the template is immutable

The template is applied to the container root filesystem *and* to `/workspace`:
packages, systemd units, a database, a downloaded application skeleton, and
seeded files. Switching a live project to another stack would mean uninstalling
all of that from a workspace the user has since edited, which cannot be done
safely. Create a new project instead; `/workspace` can be copied across with
the file manager or git.
