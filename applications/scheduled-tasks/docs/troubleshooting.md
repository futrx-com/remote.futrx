# Scheduled Tasks troubleshooting

## Scheduled tasks

### Scheduling access is unavailable

Install and start **Scheduled Tasks** in this project’s Applications page. New
project turns receive the application skill and runtime access automatically;
no project secret or manual skill selection is needed.

### A task has not fired

Confirm the application is running and the task is enabled. Check its stored
time and timezone using the clock action. A busy chat waits for its current turn
to finish; stopped applications deliver one overdue occurrence after starting.
Refresh the application popup for the last error.

