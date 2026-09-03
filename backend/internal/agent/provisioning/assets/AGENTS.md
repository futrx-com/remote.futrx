# Sandbox: remote.futrx

You're running inside an unprivileged LXC container, one per project,
spawned by [remote.futrx](https://{{PUBLIC_HOSTNAME}}). Other projects do
not share this filesystem or process namespace: fresh `apt` installs,
crashed processes, and deleted files stay within this project.

## Filesystem

- `/workspace` - your project files. Persistent, survives container
  restarts and reprovisions.
- `/root/.codex`, `/root/.claude`, `/root/.kimi-code`, and
  `/root/.gemini/antigravity-cli` - persistent, project-specific provider
  homes. The host mounts and manages these paths so provider configuration,
  authentication, and session state survive container replacement. Only the
  Antigravity subdirectory is mounted, not all of `/root/.gemini`. Keep project
  artifacts in `/workspace`, not in these homes.
- `/root/.claude/CLAUDE.md` AND `/root/.codex/AGENTS.md` - this file
  (identical content, two paths so both Claude and Codex pick it up).
  The host re-pushes both whenever the template changes; don't edit
  them expecting changes to stick.
- Everything else outside the five durable mounts: replaceable.

## Capabilities

- Root in the container (uid 0). Unprivileged means container-root maps to a
  low-privilege host user rather than host root. Do not intentionally access
  the host or another project; the current shared network does not enforce
  complete lateral separation between containers.
- `apt-get install` whatever you need.
- Network is fully open.
- `systemd` is PID 1. Use transient systemd services for dev servers and other
  processes that must survive the agent command that launches them.
- The container remains alive between prompts, but processes launched inside an
  agent-managed shell or PTY are not durable: the runner may terminate them
  when that execution ends. `&`, `nohup`, and `disown` are not sufficient for
  a server the user must continue using. Run persistent dev servers as systemd
  services. Services still stop on container stop/reboot, and transient
  services are not recreated after boot.

## Pre-installed tools

`git`, `gh`, `openssh-client`, `jq`, `build-essential`,
`python3` + `pip`, `node 22` + `npm`, `claude`, `codex`, `kimi`, `agy`. Anything else:
`apt-get install` or `npm i -g` freely.

**Persistence rule.** `/workspace/**` and the four provider homes listed
above are host bind-mounts and survive container replacement. Other paths
(`/usr/local/`, unmounted paths under `/root/`, and packages you apt-install)
are gone if the container is recreated. If you install a tool the project
needs again later, append the install line to `/workspace/setup.sh` so a fresh
container can rebootstrap with `bash /workspace/setup.sh`.

## Secrets

Tokens are project-scoped. Ones the user has configured are exported as
env vars *and* mirrored to `/workspace/.env`. CLIs that read env (`gh`,
`wrangler`, `hcloud`, `aws`, …) pick them up automatically — no
`source`, no `--token` flag, nothing.

Discover what's currently set:

```bash
env | cut -d= -f1 | grep -E '_(TOKEN|KEY|SECRET|PASSWORD)$|^(GITHUB|CLOUDFLARE|HCLOUD|OPENAI|ANTHROPIC|AWS|GOOGLE)_' | sort
# Names from the persisted dotenv file, without printing secret values:
sed -nE 's/^(export )?([A-Za-z_][A-Za-z0-9_]*)=.*/\2/p' /workspace/.env 2>/dev/null | sort -u
```

Never print, `cat`, or broadly inspect `/workspace/.env`, `/proc/*/environ`, or
credential-bearing environment values merely to discover configuration. Read
only the specific non-secret value required for the task. Project routing
metadata must come from the documented project-slug value below, never from
OAuth redirects, tokens, or other credentials.

If you need a credential that isn't set, ask the user to add it via **this
project's Containers → Secrets** in the web UI. Use the canonical
env-var name the upstream CLI expects — never invent your own. Common
ones:

| Provider | Env var | Generate at |
|---|---|---|
| GitHub (git clone / push) | `GITHUB_SSH_KEY` (paste the **private** key — full PEM, including BEGIN/END lines) | https://github.com/settings/keys → "New SSH key" (paste the matching public key) |
| Cloudflare | `CLOUDFLARE_API_TOKEN` | https://dash.cloudflare.com/profile/api-tokens |
| Hetzner Cloud | `HCLOUD_TOKEN` | console.hetzner.cloud → Security → API Tokens |
| OpenAI | `OPENAI_API_KEY` | https://platform.openai.com/api-keys |
| Anthropic | `ANTHROPIC_API_KEY` | https://console.anthropic.com/settings/keys |
| AWS | `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY` | IAM → Users → Security credentials |
| Google Cloud | `GOOGLE_APPLICATION_CREDENTIALS_JSON` (service-account JSON, raw) | IAM → Service Accounts → Keys |
| npm registry | `NPM_TOKEN` | https://www.npmjs.com/settings/~/tokens |

New values are live on your *next* shell. Already-running processes
(dev servers you started earlier, etc.) keep their old environ — kill
and restart them to pick up new credentials.

A systemd service does not automatically inherit every variable from the
current agent shell. If a server needs an exported variable, pass only the
required variable by name with `systemd-run --setenv=NAME`, repeated once per
variable, or use the application's normal dotenv loading. Do not place literal
secret values in commands, unit descriptions, process arguments, or logs.
Restart the service after credentials change.

## Dev servers — start durably and hand back the routed URL

Whenever the user asks for a dev server, **the URL they reach it at
is**:

```
https://<this-project-slug>--<port>.dev.{{PUBLIC_HOSTNAME}}
```

Replace `<this-project-slug>` with the project slug shown in the environment
context and `<port>` with the port the application uses. Do not guess or derive
the slug from OAuth configuration, repository names, or user data.

`localhost:<port>` is useful for health checks inside the container, but never
give it to the user: they are on another machine. Give them the routed HTTPS
URL. The route may be protected by the remote.futrx login, and its certificate
is issued automatically on first access.

### Before starting

1. Choose the intended HTTP port and check whether it is already owned:

   ```bash
   ss -ltnp '( sport = :<port> )'
   ```

   If a listener exists, identify it. Reuse it when it is the expected healthy
   application; otherwise choose another port or stop only a service known to
   belong to the current task. Never kill an unrelated listener.

2. Bind the web server to `0.0.0.0`, not `127.0.0.1`, so the LXC bridge can
   reach it. Keep databases, Redis, and other private infrastructure on
   loopback unless the user explicitly needs them exposed.

3. Configure the framework's documented host/origin allowlist for the exact
   routed hostname
   `<this-project-slug>--<port>.dev.{{PUBLIC_HOSTNAME}}`, or the suffix
   `.dev.{{PUBLIC_HOSTNAME}}` when the framework documents a suffix form.
   Syntax varies by framework and version. Do not disable host validation
   wholesale.

### Start the server durably

A server that must outlive the current tool call must run as a transient
systemd service, not in a managed PTY and not with `&`, `nohup`, `disown`, or
`systemd-run --scope`. Use a deterministic, sanitized unit name, an absolute
working directory, and the real executable:

```bash
systemd-run \
  --unit="<app>-dev-<port>" \
  --description="<app> dev server on port <port>" \
  --collect \
  --service-type=exec \
  --working-directory="/workspace/<project>" \
  --property=Restart=on-failure \
  --property=RestartSec=2s \
  /usr/bin/npm run dev
```

Use the project's actual command; the npm command above is only an example.
Configure host and port through the framework's supported flags or project
configuration. Add `--setenv=NAME` before the executable for each required
exported variable, without putting its value in the command.

### Verify before reporting success

Starting a command, seeing a framework print “ready,” or receiving a PID is not
proof that the server survived. After the launch command has returned, verify
the service from a separate tool call:

```bash
systemctl is-active "<app>-dev-<port>"
systemctl status "<app>-dev-<port>" --no-pager --full
ss -ltnp '( sport = :<port> )'
curl -fsS "http://127.0.0.1:<port>/<expected-route>"
journalctl -u "<app>-dev-<port>" -n 100 --no-pager
```

Confirm all of the following:

1. The unit is `active (running)` and is not crash-looping.
2. The expected process is listening on `0.0.0.0:<port>`, not only
   `127.0.0.1:<port>`.
3. The expected application route succeeds locally and, when practical,
   returns application-specific content rather than merely any HTTP 200.
4. The routed HTTPS hostname responds through the public ingress. An
   unauthenticated `302` redirect to the remote.futrx login confirms
   ingress/auth routing only; it does not prove the application itself is
   healthy. Use the authenticated browser when an end-to-end check is needed.
5. Recent service logs contain no startup failure or restart loop.

Useful lifecycle commands:

```bash
systemctl restart "<app>-dev-<port>"
systemctl stop "<app>-dev-<port>"
journalctl -u "<app>-dev-<port>" -n 100 --no-pager
```

In the handoff, give the routed HTTPS URL, systemd unit name, port, expected
route, and checks actually performed. Do not claim the server is reachable
based only on its startup output.

### "Create a site" means create it here — not on third-party hosting

When the user asks you to build a site or web app — static, full-stack,
anything — build it in `/workspace` and serve it from this container.
The dev-server URL above is available through the externally routed project
URL, which may be access-controlled: that is the link you hand back, and no
external hosting is required. Full-stack works here too — run the backend on
another intentional HTTP port and `apt-get install` postgres/redis/whatever
you need, while keeping private infrastructure bound to loopback.

**Do not deploy to third-party platforms** (Vercel, Netlify, Cloudflare
Pages/Workers, GitHub Pages, Render, Fly.io, Railway, Firebase, …)
unless the user explicitly asks for that service. External platforms
mean accounts, tokens, DNS, and billing the user didn't ask to touch.
If you think the project has outgrown the dev server, say so and let
the user decide — never sign up for or provision outside hosting on
your own.

### Multi-line secrets (SSH keys, JSON service accounts, PEM certs)

The Secrets value box accepts newlines — paste the whole PEM block, the
whole JSON service account, the whole PKCS#1 blob. The value reaches
this container as a single env var with the newlines intact, and lands
in `/workspace/.env` with the newlines encoded as `\n` escape sequences
(compatible dotenv libraries decode them). Do not print the file to inspect
those values.

If you need the secret as a file (most ssh / gcloud / certbot use
cases), write it yourself — don't try to source `.env` for those, the
escape encoding will burn you:

```bash
# SSH private key
mkdir -p /root/.ssh && chmod 700 /root/.ssh
printf '%s\n' "$GITHUB_SSH_KEY" > /root/.ssh/id_ed25519
chmod 600 /root/.ssh/id_ed25519
ssh-keyscan github.com >> /root/.ssh/known_hosts 2>/dev/null
git clone git@github.com:org/repo.git    # works

# GCP service account
printf '%s' "$GOOGLE_APPLICATION_CREDENTIALS_JSON" > /root/gcp-key.json
export GOOGLE_APPLICATION_CREDENTIALS=/root/gcp-key.json
```

`printf '%s' "$VAR"` preserves the in-memory value byte-for-byte. Avoid
`echo` for binary-ish content — some shells interpret backslash escapes.

When you need a secret that isn't there yet, tell the user the exact
canonical name (e.g. `GITHUB_SSH_KEY`, `GOOGLE_APPLICATION_CREDENTIALS_JSON`)
and that they should paste the value — including newlines — into the
project's **Containers → Secrets** UI.

## Browser automation

There are two browser modes:

- **Live Agent Browser**: use the `browser` skill for sites where the user
  logs in through the Browser pane and the agent drives that same Chromium
  session through MCP/CDP.
- **Headless utility browser**: use `/workspace/scripts/browser.mjs` for
  public URLs or sites authenticated by copied cookies.

### Headless utility browser

The platform ships a generic Playwright wrapper at
**`/workspace/scripts/browser.mjs`**. Use it to screenshot or record public /
cookie-authenticated pages:

```bash
node /workspace/scripts/browser.mjs screenshot https://app.example.com/dashboard
node /workspace/scripts/browser.mjs record     https://app.example.com/flow --duration 8000
```

Output paths print on stdout; files land in `/workspace/.browser/`. Use
your `Read` tool on PNGs; videos are `.webm`.

The script reads **`/workspace/.agents/browser-auth.json`** to know which
cookie to attach for each host. To add a new site:

1. Add one entry to the JSON yourself (the file is empty by default):
   ```json
   {
     "app.example.com": {
       "cookies": [
         { "name": "<cookie-name>", "domain": "<cookie-domain>",
           "secret": "<ENV_VAR>", "path": "/",
           "httpOnly": true, "secure": true, "sameSite": "None" }
       ]
     }
   }
   ```
   `secret` is the **name** of an env-var (e.g. `LINEAR_SESSION`) — never
   put the cookie value into this file.
2. Ask the user to paste the cookie value into **Containers → Secrets**
   under the env-var name you chose. Tell them which cookie to copy:
   *DevTools → Application → Cookies → `<cookie-domain>` →
   `<cookie-name>` → copy the Value column.*

**Don't type passwords or complete "Sign in with Google / Apple" flows
headlessly** — automated browsers are detected and refused. For sites that
need a real login, use the **Live Agent Browser** below: the user logs in by
hand once, then you drive that same authenticated session.

**If a script suddenly returns a logged-out page**, the cookie has
rotated. Tell the user to re-paste a fresh value — don't silently retry.

### Live Agent Browser — driving the browser the user logs into

For tasks that need a **real browser the user logs into** (social media,
dashboards, any authenticated site), use the **`browser` skill**: it gives
you `browser_*` tools wired to a live Chromium the user can watch and log
into, and carries the full playbook (reading via snapshots, the login
handoff, the write-approval policy). The user selects it for the request.
This is the only path for login flows that need passwords, 2FA, or provider
OAuth pages.

### Recording headless flows (clicking, filling, multi-step)

When the user asks you to record a click sequence, fill out a form, or
demonstrate a multi-step interaction, write a recipe and let the generic
script drive it:

```js
// /workspace/.browser/recipes/<scenario>.mjs
export default async function (page, context) {
    await page.goto('https://app.example.com/dashboard');
    await page.waitForLoadState('networkidle');
    await page.click('text=Analytics');
    await page.waitForTimeout(1500);
    await page.click('text=Reports');
    await page.waitForTimeout(1500);
    // optional: return a value, it'll print as JSON on stdout
    // return { title: await page.title() };
};
```

Then invoke it via the **`run`** subcommand:

```bash
node /workspace/scripts/browser.mjs run /workspace/.browser/recipes/dashboard-tour.mjs --record
```

The generic script handles the cookie setup, viewport, video recording,
cleanup, and path-printing. **Every cookie from every entry in
`browser-auth.json` whose secret is set gets attached up-front**, so the
recipe can navigate to any registered site without you having to specify
which.

Flags:
- `--record`        record a `.webm` of the run (omit for headless action only)
- `--out <path>`    where to write the video (default: `/workspace/.browser/<auto>.webm`)
- `--timeout <ms>`  abort the recipe if it runs longer (default 300000 = 5 min)

Recipes are throwaway — put them in `/workspace/.browser/recipes/` and
delete them when you're done with the scenario. **Don't reach for a
recipe for a single screenshot** — that's what the `screenshot` and
`record` subcommands are for.

### Playwright install

If Playwright isn't installed yet, `scripts/browser.mjs` will print the
one-time install command — run it, then retry.

## Project skills

Project-scoped skills have one source of truth:

- `/workspace/.agents/skills/<name>/SKILL.md`

The host provisions that directory at launch and before each prompt. It also
keeps compatibility symlinks so Claude and legacy Codex paths resolve to the
same files:

- `/workspace/.claude/skills -> ../.agents/skills`
- `/workspace/.codex/skills -> ../.agents/skills`

When suggesting that the user create a new project skill, use the
`/workspace/.agents/skills/` location. Never duplicate the same skill into
`.claude/` or `.codex/`.
