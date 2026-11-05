# Kimi runtime support — 2026-09-06

The integration targets the pinned `@moonshot-ai/kimi-code` 0.40.1. It uses
Kimi's [authenticated loopback server](https://moonshotai.github.io/kimi-code/en/reference/server-api.html)
instead of the lossy print stream. The
original review covered Remote at `7036dfdf`; repository issue searches found
feature requests but no report identifying the endpoint behind the user's 307.
PR #89 and its revert, PR #122, concerned the old mode/effort advertisement.

## Runtime and feature coverage

Remote starts `kimi web --no-open --host 127.0.0.1 --port 0` through an embedded
Node 22 stdio bridge. The bridge runs in the same host/project execution scope
as Kimi. Its server token and startup URL stay private; Remote receives JSON
responses and events. Neither the server nor its debug endpoints is exposed
through the project's public ingress. The provider home, credentials, session
history, custom agent definitions, skills, hooks, plugins, and user MCP config
remain Kimi-owned.

| Feature | Remote behavior |
| --- | --- |
| Models | Native model catalog, exact aliases, configured defaults, model input modalities and supported reasoning controls. Auto resolves the current configured default. |
| Thinking | Applies the chosen effort to the session and streams main/child reasoning. Auto resolves native configured/model defaults. |
| Plan | Native session Plan mode, plan review choices, approval, revision feedback, rejection and cancellation. |
| Permissions | Manual, Ask when needed (`yolo`), Never ask (`auto`); correlated approval and session-scope responses. Kimi's names differ from the apparent meaning of `--yolo`. |
| Questions | Main and child requests, multiple questions, single/multiple selections, option IDs, free text, dismissal and late-response handling. Resolution events omit answer payloads. |
| Delegation | All-agent subscription: foreground/background agents, swarms, nested parent IDs, tool activity, reasoning, reports and terminal states. Child output stays in child cards. |
| Agent controls | Stop and detach supported main-owned tasks from their child cards. Cancelling the whole Remote run aborts the main turn, cancels tasks and shuts down the server/process group. Nested tasks remain controlled by their owning native agent. |
| Completion | Waits for main completion, pending interactions, live tasks, active goals and scheduled work; confirms idle twice so task callbacks can enqueue follow-up turns. No completion on server loss or failed/cancelled turns. |
| Usage | Adds disjoint per-step input/cache/output buckets from all agents once. Child cumulative summaries and replayed steps are not added again. Native prices/reasoning-token splits are not invented. |
| Resume/fork | Native persisted resume and context-preserving fork. Only an initial session-lookup 404 triggers Remote's visible-history recovery; execution failures are never automatically replayed. |
| Custom agents/skills | Native discovery from Kimi's normal directories, explicit profile binding for a new session, bundled skill activation and Remote's selected-skill instructions. |
| Side questions | `/btw` forks a native child with tool execution disabled; answers stay in its card and do not enter the main agent's working context. |
| MCP/browser | Existing MCP config loads normally. Selecting Browser prepares the existing browser runtime and merges a dedicated `remote_browser` entry through Kimi's API. Runs without Browser disable that entry's tools. Other registry entries are preserved. |
| Hooks/plugins | Loaded and executed by Kimi from its normal configuration; native lifecycle and extension events are retained. Configuration and management remain available through native Kimi tools/interfaces rather than duplicate Remote admin screens. |
| Goals/cron | Native goal controls and continuation. CronCreate/List/Delete results and fire events keep the run open; IDs/recurrence flags are mirrored in session metadata for Remote resumes. Native recurring jobs keep the run active until cancelled. Remote's durable scheduled-task tools remain available. |
| Context | Automatic/manual compaction and native undo; context snapshot contents are omitted from telemetry because text/tools already represent the transcript and snapshots can repeat private answers/media. |

Use `/kimi help` in a Kimi chat for controls:

- `/agent <profile>` followed by the prompt on the next line. Kimi binds a
  profile once per session; start a new chat to choose a different profile.
- `/swarm on` or `/swarm off`, optionally followed by a prompt on the next line.
- `/tower on` or `/tower off` where enabled by the pinned CLI.
- `/btw <side question>` for a text-only native side conversation.
- `/goal <objective>`, `/goal pause`, `/goal resume`, `/goal cancel`.
- `/compact [instructions]`, `/undo [turn count]`.
- `/skill:<name> [arguments]`.

Commands are recognized only in the current user input, before history/skill
enrichment. Normal prompts retain that enrichment. The composer provides model,
Thinking, Plan and approval controls; it does not offer Kimi an OS sandbox it
cannot enforce. Kimi CLI utilities and native settings interfaces (doctor,
export, migration, visualizer, plugin/provider/MCP account management) retain
normal CLI behavior; this adapter is not a reproduction of every TUI screen.

## Implementation ownership

The provider prepares the execution scope and collects credentials. The run
coordinates the protocol through explicit owners:

| Responsibility | Owner |
| --- | --- |
| Bridge startup and resource limits | `bridge.mjs` exports `startKimiBridge`; `command.go` injects `config/constants/kimi.go` values. Importing the bridge does not start Kimi. |
| Process lifecycle and terminal result | `server_process.go` starts, aborts, drains and closes the transport before outcome publication. Shared line-based providers use `runtime/stderr.go` for bounded diagnostic capture. |
| Session identity and configuration | `server_session.go`, `server_preferences.go`, and `browser_mcp.go` establish identity, translate composer preferences and prepare the dedicated MCP entry. `server_requests.go` owns private native request DTOs, including explicit omitted/empty/false distinctions. |
| Event routing | `server_events.go` handles session/replay filtering and routes native events. `server_protocol.go` contains the private event decoder shapes. |
| Delegated activity | `server_activity.go` owns agent ancestry, tool correlation, child transcript state, task ownership, terminal reconciliation and collaboration projections. |
| Usage and scheduled lifetime | `server_usage.go` owns per-step deduplication and totals; `server_cron.go` owns tracked cron jobs and metadata persistence. |
| Interactions | `server_interaction_requests.go` owns pending IDs and public projections; `server_answers.go` validates replies without I/O; `server_interactions.go` and `server_task_controls.go` perform native operations and resolve acknowledged requests. |
| Model declarations | `model_traits.go` maps native thinking/modality declarations; `capability_parser.go` retains catalog shape, alias and override compatibility. |
| Chat forms and controls | `questionAnswerState.ts` owns component-local answer transitions, exposed through `useQuestionForm`. UI emits typed intents; `chatInteractionService` encodes native replies. Delegated control targets reach the card through `useDelegatedAgentControls`. |

The refactor preserves native request ordering, error messages, event ordering,
replay handling, idle polling, child-delta throttling, private-answer filtering,
and the existing form keys, labels and selection semantics. Registration,
provisioning contracts, shared event models and prompt enrichment remain in
their existing layers.

## Separate follow-up found during refactoring

`server_commands.go` uses a literal space to separate a slash command from its
arguments, while the preference preflight's `userCommand()` uses whitespace
fields. Consequently `/agent<TAB>profile` skips normal profile preferences but
is submitted as ordinary prompt text. Use the documented space syntax. This
inconsistency was left for a separate behavior change; it is not silently fixed
by the structural refactor. Rapid repeated option clicks sharing a render also
retain their existing selection semantics, covered by the local state test.

## The 307 report

The installed CLI is exercised through the actual adapter against a loopback
mock provider with an isolated home and dummy key:

- A 307 with a valid relative `Location` is followed as POST and completes.
- A 307 without `Location` fails with `provider.api_error: 307 status code
  (no body)`. Remote shows that diagnostic and does not emit completion.

HTTP 307 and process exit status are different. These checks reproduce a failure
path, not the unknown endpoint/proxy behind the original incident. No endpoint,
credential, redirect policy or automatic prompt retry was changed speculatively.
Startup/transport failures retain bounded diagnostics; stream loss is explicit
and requires resume rather than risking duplicate tool execution.

## Validation and limits

From `backend/`:

```sh
go test ./...
go test -race ./internal/integration/agents/kimi ./internal/agent ./internal/service/prompt ./internal/service/agent/capability
REMOTE_KIMI_SMOKE=1 go test -v ./internal/integration/agents/kimi -run TestInstalled -count=1
```

Installed-CLI checks cover redirects, reasoning, usage, questions, permissions,
foreground/background delegation, swarms, cancellation, stopping one agent,
resume/fork, Plan alternative approval, isolated side questions, goals, custom profiles/skills, native model discovery and
browser MCP with a fake MCP server. Deterministic protocol tests additionally
cover nested correlation, duplicate usage, plan feedback, stale answers,
recovery boundaries, background completion and cron lifetime bookkeeping.
All 250 frontend tests pass, including capability selection/approval separation
and response encoding. The production build and browser checks cover question
selection/other text, secret-input trimming, dismissal, plan feedback/choices,
and delegated stop/background controls. The browser checks were also run against
the original question form before comparing the extracted state implementation.
`go vet ./...`, `go build ./...`, and the race tests (including shared runtime)
pass after the refactor.

This is host and container-command validation, not a deployed QA LXC run with a
live subscription. Authentication presence checks do not prove account
entitlement or endpoint availability. Kimi's server API is evolving: upgrade
the pin and compatibility tests together. Native cron has no public REST list
endpoint in this version; cron edits made outside Remote require a native
CronList refresh to reconcile Remote's mirrored IDs. Persistent scheduling
without an active chat should use Remote's scheduled-task tools.
