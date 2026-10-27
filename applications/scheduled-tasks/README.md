# Scheduled Tasks

Scheduled Tasks lets you ask the agent to do work later or repeat it on a schedule. When a task is due, the application sends your saved instructions to the agent in the same project chat. The agent carries out the request and replies there, just as it would during an ordinary conversation.

Use it for reminders, recurring reviews, or checks that stop when a goal is reached. You can close the browser after scheduling a task. The scheduler runs on the Remote host and can wake a stopped project container when work is due.

## What you can do

| Capability | Example request |
| --- | --- |
| Schedule work once | “Tomorrow at 15:00 UTC, review build build_123 and tell me whether it passed.” |
| Repeat work on a schedule | “Every day at 15:00 UTC, remind me to review the deployment.” |
| Monitor a goal for a limited time | “Check the deployment every five minutes, for up to 12 checks. Stop early if it is healthy and report the result.” |
| Manage existing tasks | View tasks in the chat, pause or resume them, request an immediate run, archive them, or delete them. |

The application schedules agent work. What the agent can actually do depends on the tools and access available in your project. Include the target, the expected result, and any stopping condition in your request.

## Get started

1. Open your project's **Applications** page and install **Scheduled Tasks**. Make sure the application is running.
2. In a project chat, ask the agent to schedule the work. Include a time and timezone, or a recurring schedule.
3. Check the agent's confirmation for the task ID, scheduled time, and timezone. New tasks are active immediately.
4. Select the **Scheduled Tasks** calendar icon with a small clock badge in the chat header to see the saved task and its status.

For example:

> Every day at 15:00 UTC, remind me to review the deployment.

You do not need to select a skill manually or activate the task in a separate step. If the timezone is unclear, the agent should ask you to clarify it before creating the schedule.

## Manage tasks in the chat

The **Scheduled Tasks** calendar icon with a small clock badge opens the current chat's task list. Each task shows what the agent will do, its schedule, and its current status. The open panel refreshes automatically every three seconds; you can also use **Refresh**.

| Action | What it does |
| --- | --- |
| **Pause** | Stops future automatic runs. A run already queued or executing can still finish. |
| **Resume** | Activates a paused task and calculates its next future run. Missed intervals are not replayed. |
| **Run now** | Requests one run immediately. If a recurring task is active, its next scheduled time is calculated after this run finishes. A paused task stays paused. |
| **Archive** | Stops future automatic runs and keeps the task definition in the archived list. A run already queued or executing is not cancelled. |
| **Restore** | Returns a manually archived task to the paused list. Use **Resume** to activate it again. |
| **Delete** | Removes the saved task definition. |

Finished tasks move to the archive automatically. Select **Show archived tasks** to see them. The archive retains task definitions and status; the agent's replies remain in the chat.

A **Run pending** status means the task is waiting for the agent or is already running. The panel updates as the run progresses and shows the last error when one is available.

Tasks are created through conversation; there is no form for editing a task's instructions or schedule. To change either, ask the agent to create a replacement and remove the old task. A one-time task whose scheduled time has passed cannot be resumed, and a task that has reached its run limit cannot be resumed. Finished tasks must be scheduled again to get more automatic runs.

## How schedules work

### One-time tasks

A one-time task runs at a specific future date and time. Internally, its time is stored in RFC3339 format with an explicit UTC offset, such as `2026-12-01T15:00:00Z`. You can describe the time naturally in the chat; the agent converts it to this format.

### Recurring tasks

Recurring tasks use a five-field cron expression and a named timezone, such as `UTC` or `America/Toronto`. You can ask for a schedule in plain language without writing cron yourself.

The fields are `minute hour day-of-month month day-of-week`:

| Expression | Schedule |
| --- | --- |
| `0 15 * * *` | Every day at 15:00 in the selected timezone |
| `*/5 * * * *` | Every five minutes |
| `0 9 * * 1-5` | Monday through Friday at 09:00 in the selected timezone |

Numeric lists, ranges, and steps are supported. Sunday can be written as `0` or `7`. If both day-of-month and day-of-week are restricted, either condition can trigger a run, following traditional cron rules.

### Run limits and stopping conditions

A recurring task can stop after a fixed number of runs or when the agent determines that its goal is complete. For monitoring requests, specify both a stopping condition and a maximum number of checks unless you want the task to repeat indefinitely.

Once the goal is complete, the scheduled agent can mark its own task complete, stopping future runs while retaining the definition. One-time tasks and tasks that reach their run limit are also archived automatically. The run limit counts finished agent turns, including failed turns.

## Timing and recovery

A scheduled time is when the work becomes due. Execution may wait if the chat or shared agent runtime is busy. Each chat runs one agent turn at a time, and the shared application runtime allows at most two simultaneous application turns across the server.

The application saves tasks and pending runs so it can recover after a restart. If it was stopped while a recurring task became due, starting it delivers one overdue occurrence rather than replaying every missed interval. After an active recurring run finishes, the next scheduled time is calculated from the current time.

Delivery is **at least once**: work interrupted by a host restart may be retried. For requests that change an external system, the instructions should tell the agent to check whether the action has already happened before repeating it.

## Stop, start, upgrade, or uninstall

| Application action | Effect on tasks |
| --- | --- |
| **Stop** | Stops the scheduler backend and automatic delivery. Saved tasks are retained. |
| **Start** | Restarts the scheduler using its saved tasks and pending runs. |
| **Upgrade** | Retains the application's saved data. |
| **Uninstall** | Removes the installed CLI, published skill, and application instance data, including saved tasks. |

The retired built-in scheduler's data file is left untouched. Its tasks are not imported automatically. Recreate any reminders you want to keep through the installed Scheduled Tasks application.

## Limits and access

- Each project installation can store up to **100 task definitions**, including archived tasks. Delete old tasks to free space.
- Each saved prompt can contain up to **32 KiB** of text.
- Ordinary agent turns can manage tasks owned by the caller in their current chat and project. Scheduled turns can mark their own task complete; they cannot create new schedules or manage other tasks.
- Remote checks the owner's access to the chat and project before every dispatch.
- No additional project secrets, container daemon, exposed port, or external infrastructure are required.

## Developer reference

### Package layout

The package follows the Hello Remote application structure:

| Path | Responsibility |
| --- | --- |
| `application.json` | Declares installation, backend capabilities, lifecycle events, and the UI extension. |
| `backend/main.go` | Connects the API, lifecycle publisher, and `Runtime.AgentTurns` through `ServeWithRuntime`. |
| `backend/api/` | Validates tasks, persists state, calculates schedules, and manages execution. |
| `backend/lifecycle/` | Publishes the `tasks.due` event. |
| `infra/` | Installs and packages the container CLI. |
| `skills/` | Publishes the agent workflow for scheduling and completing tasks. |
| `ui/` | Adds the chat-header Scheduled Tasks calendar icon with a small clock badge and task panel through the supported extension API. |

Task definitions are stored in `tasks.json` inside the application instance's private `DataDir`.

### Agent CLI and runtime access

The installed CLI is `/workspace/scripts/remote-schedule`. It supports `create`, `list`, `pause`, `resume`, `run-now`, `delete`, and `complete-current`. The [published skill](skills/scheduled-tasks/SKILL.md) documents the commands and the agent's scheduling workflow. Use `--max-runs N` when creating a task with a fixed run limit.

Remote supplies `REMOTE_APPLICATION_API` and `REMOTE_APPLICATION_GRANT` to eligible agent turns and revokes the grant after each run. These values are temporary runtime capabilities, not project secrets to configure manually.

### Execution and recovery

The manifest opts into shared background recovery, agent turns, and agent tools. Remote restores running backends that opt into recovery after host restarts or backend crashes.

Before calling `AgentTurns.Start`, the application saves a durable claim identifying the pending run. It retries busy execution every 15 seconds and polls accepted turns once per second. The `tasks.due` event is for observability; it does not acknowledge delivery.

The application interprets completion markers, saves the outcome and next deadline, then asks Remote to forget the durable execution receipt. A backend restart reuses the existing turn; an interrupted agent may be retried after a host restart. See the [generic runtime contract](../../docs/dev/installable-applications/25-application-agent-runtime.md) for the shared execution behavior.

### Build and verification

After editing `infra/remote-schedule`, rebuild the CLI archive:

```sh
bash applications/scheduled-tasks/infra/build-payload.sh
```

The payload test checks that the archive matches its source. Run these checks from the repository root:

```sh
jq empty applications/scheduled-tasks/application.json
bash -n applications/scheduled-tasks/infra/*.sh
node --test applications/scheduled-tasks/infra/*.test.mjs applications/scheduled-tasks/ui/scripts/*.test.mjs
go test ./applications/scheduled-tasks/backend/...
(cd backend && go test ./internal/service/applications ./internal/service/prompt ./internal/integration/containers/applications)
```

The core integration test builds and runs the packaged backend through the generated-module builder. It creates an active task using a scoped grant, restarts the backend, checks that the correct chat receives the scheduled turn, and verifies completion.

For a manual delivery check, install the application in a disposable project, ask for a reminder one minute ahead without selecting a skill, and close the browser. Confirm that the reminder appears in the same chat. Also check stop/start behavior and that uninstall removes the Scheduled Tasks calendar icon and scheduling access.
