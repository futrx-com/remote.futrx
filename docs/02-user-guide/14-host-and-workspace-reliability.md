# Host and workspace reliability

Remote keeps the control plane on the host and gives every project its own LXD
container. This page explains the reliability behavior around resource limits,
browser previews, Git commits, host administration, and infrastructure updates.

## What this protects

| Area | Previous symptom | Current behavior |
| --- | --- | --- |
| Memory limits | Reconvergence could replace an explicit project limit with the default | An explicit override remains authoritative; the default is used only when no override exists |
| Dev-server previews | A server worked inside the container but its public preview failed or generated the wrong origin | Code-server, Vite, Caddy, and container lifecycle provisioning use the same project preview hostname |
| Git commits | VS Code reported that `user.name` and `user.email` were missing | New and recycled workspaces receive safe, non-secret author metadata in a writable global Git config |
| Host operations | An administrator had to leave Remote and open a separate SSH session | **Settings → Terminal** opens an administrator-only terminal on the Remote host |
| Workspace updates | Container migration could stop because `BASE_URL` was unavailable outside systemd | The upgrader reads the installed public URL from the rendered service unit without loading secrets |

## Memory limits survive convergence

Project resource settings are policy, not a one-time launch hint. When Remote
starts, repairs, or replaces a container, it distinguishes an administrator's
explicit memory override from an absent value. Explicit CPU, memory, and disk
choices remain attached to that project; platform defaults fill only missing
settings.

Open the project's settings to inspect or change its limit. A workspace
replacement may stop and recreate the container, but it must not silently
restore a different memory value.

## Preview routing has one public origin

A project dev server still listens inside its container. Remote publishes it
through a dedicated HTTPS hostname and passes the same origin into code-server
and the Vite allow-list. The generated Caddy configuration forwards requests
and WebSockets without requiring a project to expose its container directly.

After changing the deployment hostname or upgrading an older container, Remote
converges the code-server systemd drop-in. It restarts an active IDE only when
that managed configuration changed.

## Git author identity and GitHub authentication are separate

Git needs author metadata before it can create a commit. Remote provisions the
following safe defaults when no operator override is supplied:

```text
user.name=Remote User
user.email=remote@localhost
```

Operators can set `REMOTE_GIT_USER_NAME` and `REMOTE_GIT_USER_EMAIL` in the
host-local `config.env`. That file is not part of the repository.

The provisioner writes only these two non-secret fields. GitHub tokens, SSH
private keys, passwords, and credential helpers are never copied into Git
configuration by this feature. GitHub may still ask the user to authenticate
the first time a workspace pushes; that authorization is independent of the
commit author identity.

Some provider credential mounts make `/root/.config/git` read-only. Remote
therefore writes author metadata to `/root/.gitconfig`. The code-server service
receives explicit `HOME=/root` and `GIT_CONFIG_GLOBAL=/root/.gitconfig` values
so the terminal and VS Code Git extension resolve the same identity.

## Use the global terminal carefully

Server administrators can open **Settings → Terminal** to work on the Remote
host rather than inside one project. The terminal attaches to a server-selected
tmux session and the WebSocket endpoint requires an authenticated administrator.
A client cannot choose an arbitrary host session.

The global terminal has host-level reach. Use a project terminal for normal
development and reserve the global terminal for operations such as service
status, disk inspection, backups, and controlled updates. Commands run there
can affect every project.

## Infrastructure updates keep secrets out of the shell

The application service receives its public `BASE_URL` from the rendered
systemd unit. Workspace migration runs from an operator shell, so it does not
automatically inherit that environment. The upgrader recovers only the
installed `BASE_URL` from the unit when needed. It deliberately does not source
`config.env`, because that file can contain credentials.

Changes to Caddy, systemd, workspace provisioning, the reusable base image, or
the updater require a minor or major release and the full infrastructure update
path. During container migration:

- `/workspace` and managed provider homes remain durable;
- idle containers are replaced and validated against the new image;
- containers with an active agent process are skipped unless the operator
  explicitly includes busy projects;
- packages installed elsewhere in a container root filesystem may disappear.

Take an external backup before a major host change. Durable storage protects
normal lifecycle operations, but it is not a substitute for a backup.

## Troubleshooting checklist

If VS Code still reports a missing Git identity:

1. Reload the IDE after code-server has been reprovisioned.
2. Run `git config --global --get user.name` and
   `git config --global --get user.email` in the project terminal.
3. Treat a later GitHub sign-in prompt as repository authentication, not an
   author-identity failure.

If a preview works on `localhost` but not through Remote, verify that the server
listens on a shareable port and use the preview URL generated by Remote rather
than a container address.

If an infrastructure update stops before workspace migration, inspect the
installer log in **Settings → Updates** and rerun the full update only after
the reported host configuration problem is resolved.
