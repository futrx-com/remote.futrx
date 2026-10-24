# Configurable project agent instructions

Set `AGENT_INSTRUCTIONS_FILE` in the backend service environment to the absolute
path of an operator-owned JSON file. Keep that file outside the application
checkout, for example `/opt/remote.futrx/data/agent-instructions.json`, so updates
preserve it. The file is read at backend startup; restart the backend after an
edit. No rebuild or base-image regeneration is needed.

```json
{
  "global": "Run the relevant checks before committing. Keep secrets out of logs.",
  "providers": {
    "claude": "Keep project-specific notes in the project's CLAUDE.md.",
    "codex": "Keep project-specific notes in the project's AGENTS.md."
  }
}
```

Use a persistent systemd override (`systemctl edit remote.futrx.service`):

```ini
[Service]
Environment=AGENT_INSTRUCTIONS_FILE=/opt/remote.futrx/data/agent-instructions.json
```

After creating the file, run `systemctl daemon-reload` and
`systemctl restart remote.futrx.service`. Restrict file ownership and editing to
the host administrator. This release provides file configuration, not a settings
UI or live reload.

For each configured provider target, Remote composes:

1. Built-in container and platform guidance, including the installed hostname.
2. The `global` addition.
3. That provider's addition, when present.

Currently Claude, Codex and MiniMax declare managed instruction targets. Unknown
provider names and providers without such a target are rejected, rather than
silently accepting instructions that would never be installed. Unknown JSON
fields, unreadable files, malformed JSON, trailing JSON values and files over
1 MiB also prevent backend startup. Leaving the environment variable unset
preserves the built-in template. Remove the variable and restart to revert.

The existing publisher installs the result in provider-home files such as
`/root/.claude/CLAUDE.md` and `/root/.codex/AGENTS.md`. It republishes stale content
before project runs, including the first run after container recreation. Already-running turns are not restarted. Container diagnostics use
each provider's composed content when checking whether instructions are current.
Saved Claude/Codex account homes retain their existing links to these shared
instruction files; credentials and conversations remain isolated per chat.

Project-level instructions stay in the repository/workspace. Remote does not
replace `/workspace/AGENTS.md`, `/workspace/CLAUDE.md`, or nested project files;
the provider CLI loads those supplements using its existing instruction rules.
The operator JSON controls only project-container managed targets, not host-chat
instruction files. This is agent guidance, not an authorization boundary.
