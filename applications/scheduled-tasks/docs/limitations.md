# Scheduled Tasks limitations

## Scheduled tasks

- Recurrence resumes the same provider session and consumes context and quota.
  Use bounded `maxRuns` where appropriate.
- The project application must be installed and running. Stop pauses delivery;
  uninstall deletes instance tasks.
- Missed cron occurrences are coalesced. Claims retry after host crashes, so
  interrupted work may repeat; delivery is at least once across restarts.
- Definitions are created through the agent. The application popup manages
  existing tasks but has no definition editor.
- Legacy built-in task definitions remain on disk and must be recreated through
  the application.

