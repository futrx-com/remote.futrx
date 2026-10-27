---
name: scheduled-tasks
description: Remember work for a specific time or repeat it on a cron schedule in this project chat.
---

# Scheduled Tasks

Use `/workspace/scripts/remote-schedule` only when the user explicitly asks for
future or recurring work. The installed Scheduled Tasks application owns the
clock and sends the stored message back to this chat as a normal agent turn.
Do not create container cron jobs, systemd timers or background agent loops.

Resolve the requested time and timezone from the user's words and project
context. Ask if the timezone is ambiguous. Store a self-contained prompt that
says what to do, what to report and when a recurring goal is complete.

One-time work uses RFC3339 with an explicit offset:

```sh
/workspace/scripts/remote-schedule create --name "review build" \
  --prompt "Review build build_123 and report its result." \
  --at "2026-10-07T15:00:00Z" --timezone UTC
```

Recurrence uses five fields (minute hour day-of-month month day-of-week) and
an explicit IANA timezone:

```sh
/workspace/scripts/remote-schedule create --name "daily reminder" \
  --prompt "Remind me to review the deployment." \
  --cron "0 15 * * *" --timezone UTC
```

New tasks are active immediately. Read the returned JSON and report the task
ID, time and timezone. Do not claim creation succeeded after a CLI error.
Use `--max-runs N` for bounded monitoring unless unlimited recurrence was requested.

```sh
/workspace/scripts/remote-schedule list
/workspace/scripts/remote-schedule pause TASK_ID
/workspace/scripts/remote-schedule resume TASK_ID
/workspace/scripts/remote-schedule run-now TASK_ID
/workspace/scripts/remote-schedule delete TASK_ID
```

During a scheduled turn, use `remote-schedule complete-current` only after the
standing goal is complete. This stops future fires and retains the task.
Scheduled turns have only that completion capability; they cannot create new
schedules. Ordinary turns can manage only this chat's tasks owned by the caller.

If scheduling access is unavailable, ask the user to install and start
Scheduled Tasks from this project's Applications page. Do not ask for secrets:
Remote supplies the API URL and short-lived capability for each eligible turn.
Stopping the application pauses all delivery; starting it retains and resumes
its tasks. Uninstalling removes the application's task data.
