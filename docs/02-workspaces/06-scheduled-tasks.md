# Scheduled tasks

Scheduled Tasks is an optional project application under
[`applications/scheduled-tasks/`](../../applications/scheduled-tasks/). It owns
the clock, cron rules, definitions, claims, persistence, retries, completion,
CLI, skill and chat-header UI. Remote supplies the same agent execution and
application tool capabilities available to other applications.

## Create and wake

1. Installing the app publishes `/workspace/scripts/remote-schedule` and its
   `scheduled-tasks` skill through normal application provisioning.
2. Supported turns with an eligible running installation receive the app's
   skill and `REMOTE_APPLICATION_API` / `REMOTE_APPLICATION_GRANT`. Explicit
   skill selection is unnecessary. The grant is scoped to the turn and revoked
   after it, with a four-hour expiry as a fallback.
3. The CLI calls `/agent-api/applications/scheduled-tasks/tasks`. Remote stamps
   caller and chat context; the application restricts task management to that
   owner and chat. Newly created tasks are active.
4. The application checks deadlines once per second and durably saves a random
   run claim. It emits `tasks.due` for observers and calls the shared SDK's
   `AgentTurns.Start`, using the run claim as the idempotency request ID.
5. The applications service checks the running installation, current owner
   registration and authority, and chat/project identity, then starts an
   ordinary prompt turn. Provider selection, session, container startup,
   transcript, usage, notifications and one turn per chat use the existing path.
6. The application polls the execution receipt. It interprets completion
   markers, records the outcome, and schedules the next occurrence or archives
   a one-time, completed or exhausted task. It forgets the receipt only after
   saving its acknowledgment. Core never reads a task record or completion rule.

The generic capability lives in
[`service/applications`](../../backend/internal/service/applications/) and is
exposed by [`pkg/applications`](../../backend/pkg/applications/). Its RPC and
process adapters carry the capability; provider adapters continue to launch
agents. See [Application agent runtime](../dev/installable-applications/25-application-agent-runtime.md).

## Storage and recovery

Tasks live in `tasks.json` in the application's `Instance.DataDir`. Snapshots
use a private temporary file, file fsync, atomic rename and directory fsync.
Stop/start and upgrades retain state; uninstall removes it and the instance's
execution receipts.

The app retries busy chats and maintenance every 15 seconds and polls accepted
turns once per second. Delivery uses acknowledged SDK calls; best-effort events
are only observability. Core persists generic accepted input and execution
results separately under `DATA_DIR/application-turns/`. A child restart reuses
an existing running or completed receipt. A Remote restart retains completed
receipts; an interrupted turn can be retried, so delivery is at least once.
Missed cron intervals produce one overdue run, then a future deadline.

The manifest opts into background recovery. Remote restores any running backend
with `backend.background`, including after child crashes, on a 15-second sweep.
Stopped installations remain stopped. The application owns what its restored
worker does.

The retired scheduler's shared data file is left intact for reference. Existing
definitions are not imported automatically; recreate wanted reminders through
the application. The former core scheduler, schedule bridge, task store, user
schedule routes and built-in drawer are removed.

## Permissions and limits

Interactive calls can manage only the owner's tasks in the stamped chat.
Background scheduler turns can complete only the task/run encoded in their
Remote-stamped opaque context. That permission is enforced by the application,
including the live claim check. Core provides a general background-origin
context and restricts a background turn's tools to its own installation.

Definitions support RFC3339 one-time dates, numeric five-field cron and IANA
timezones. Prompts are capped at 32 KiB and an installation retains 100 task
records, including archived tasks. Recurring monitoring supports `maxRuns`.
Application-initiated turns share a server limit of two concurrent executions.
There is no manual arm step or definition-edit form.

Verification covers application scheduling and permission tests, publisher
contracts, CLI payload, UI visibility, generic runtime authorization and
receipt recovery, and generated-process tests for both an independent app and
the packaged scheduler. See the [application README](../../applications/scheduled-tasks/README.md).
