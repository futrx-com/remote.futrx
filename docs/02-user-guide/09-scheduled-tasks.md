# Scheduled tasks

Install **Scheduled Tasks** from your project's **Applications** page, then ask
the agent to remember something for later in a project chat:

```text
Notify me every day at 15:00 UTC to review the deployment.
```

Include the time and timezone. For bounded monitoring, say when the goal is
complete and how many runs are allowed. The agent reports the stored task's ID
and timing. Tasks become active immediately; skill selection and manual arming
are not required. A fire sends the stored message back into the same chat as a
normal agent turn, even with the browser closed.

One-time reminders use an exact date/time with an offset. Recurring work uses
five-field cron (`minute hour day-of-month month day-of-week`) and an explicit
IANA timezone such as `UTC` or `America/Toronto`. There is no seconds field.

The application's clock icon appears in the chat header while installed and
running. It lists your tasks for the current chat, with refresh, pause, resume,
run now and delete actions. Pause keeps the definition; delete removes it.
Pending claims and active agent turns finish normally after a pause. To change
a definition, delete it and ask for its replacement. The agent can stop a
recurring task after its standing goal is complete using its current-task
completion command.

Stop the application to stop automatic delivery across its project. Start keeps
its saved tasks and delivers one overdue occurrence. Uninstall deletes its
tasks and tools. Reinstall starts empty. Old built-in definitions are retained
on disk but must be recreated through the application.

A busy chat waits for its current turn to finish. Missed cron occurrences are
coalesced. `maxRuns` limits total runs; a finished one-time reminder or exhausted
task cannot be resumed. Project members manage their own tasks; administrators
can manage project tasks. Each fire rechecks the owner's registration and
project access. At most two scheduled agent turns run simultaneously across
the server, and an installation keeps at most 100 task definitions.

Repeated reminders retain chat context and consume provider quota. A host crash
can retry an interrupted occurrence, so a task with external side effects should
check the target's state before repeating it. See the
[application README](../../applications/scheduled-tasks/README.md) and
[architecture](../02-workspaces/06-scheduled-tasks.md) for details.
