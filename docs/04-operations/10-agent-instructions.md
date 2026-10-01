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

## Administrator editor

Administrators can edit global and provider-specific additions under **Settings → Agents → Global agent instructions**. The editor lists the instruction file target supplied by each provider (for example `AGENTS.md` and `CLAUDE.md`). Project-owned instruction files continue to supplement the managed provider-home instructions.

Without `AGENT_INSTRUCTIONS_FILE`, settings are stored in `DATA_DIR/agent-instructions.json`, outside the source checkout. An explicit `AGENT_INSTRUCTIONS_FILE` remains authoritative and must exist at startup; the editor updates that file. The backend account needs write access to its parent directory for atomic replacement. Files are written with mode `0600`; back up this file with the server state.

Saving does **not** restart the server or interrupt running agents. Restart the backend after saving. Subsequent project provisioning publishes the newly composed instructions. The UI explicitly reports this requirement. GET/PUT `/api/admin/agent-instructions` requires an authenticated administrator; writes use the same bounded JSON and provider validation as startup configuration. Invalid replacements leave the previous file intact.
