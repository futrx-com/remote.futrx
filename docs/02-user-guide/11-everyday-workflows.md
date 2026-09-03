# Everyday workflows

These recipes combine the individual controls into complete ways of working.

## Build a new web application

1. Choose **New project** and name the workspace.
2. Wait for the project to finish provisioning.
3. Create a chat with the **+** beside the project.
4. Choose a provider, model, **Thinking**, and **Mode**.
5. In the prompt, state the outcome, constraints, and how the result should be verified.
6. Ask the agent to start the development server on `0.0.0.0` using a port from 1024 through 65535.
7. Watch streamed text and tool cards; expand a tool card when you need exact command or file details.
8. Choose **Open Browser**, refresh app discovery, and select the process and port.
9. Inspect the result in the split view.
10. Ask for tests and a Git commit.
11. Open **History** or **Open in IDE** for independent verification.

![A completed implementation visible beside the agent conversation](/assets/docs/screenshots/live-preview.webp)

## Improve a visible UI element

1. Open the project chat and choose **Open Browser**.
2. Select the running application.
3. Choose the crosshair **Inspect element** control.
4. Hover until the intended element is highlighted.
5. Click the element.
6. Confirm that the structured **Browser element** context appeared in the composer.
7. Add the desired change in plain language.
8. Send the prompt.
9. Reload the preview and inspect the result again.

This workflow gives the agent selector, text, HTML, bounding-box, style, and parent context without asking you to describe the element from memory.

## Run several agents in parallel

Parallelism happens across chats and projects, not inside one chat.

1. Create one chat for each independent workstream.
2. Give each chat a narrow goal and verification criteria.
3. Send the prompts.
4. Use sidebar spinners and unread dots to monitor state.
5. Move between chats while they run.
6. Queue at most the immediate next instruction in a busy chat.
7. Review and integrate each result before sending overlapping file changes.

![Independent chats working in parallel inside the same Remote workspace](/assets/docs/screenshots/parallel-agents.webp)

Concurrent chats in one project share the same files, processes, ports, browser profile, and Git repositories. They can race. Separate projects provide a stronger execution boundary.

## Switch provider without moving the project

1. Open the existing project chat.
2. Wait for any active run to finish or cancel it.
3. Choose **Codex**, **Claude**, **Kimi**, or **Antigravity** in the composer.
4. Recheck the model, thinking level, speed, and selected skills.
5. State what the new provider should continue or verify.
6. Send the prompt.

![The same durable project with provider controls available in the composer](/assets/docs/screenshots/20-agent-switching-controls-15m15s.webp)

The files stay in the project. Provider-native session state does not transfer
cleanly; when a provider session is missing, Remote supplies a bounded visible
transcript to a fresh session. Kimi and Antigravity forks start fresh.

Antigravity must first be signed in by running `agy` in that project's
Terminal. Exit the CLI and choose **Refresh models** in the chat picker before
selecting it. Its sign-in and session state are not part of Remote's durable
provider-home mounts.

## Schedule a monitor and walk away

1. Open the project chat that should own the future work.
2. Select **Scheduled Tasks** in **Skill set**.
3. Ask the agent to create a one-time or recurring task with an explicit
   timezone, completion condition, and maximum run count.
4. Wait for the agent to report the parked task.
5. Open **Schedules** in the chat header.
6. Review the saved prompt, timing, timezone, and limit.
7. Select **Arm**.
8. Close the browser if desired. The host scheduler owns the timer.
9. Return later to the same chat to inspect each scheduled run in the normal
   transcript.

Pause the task when monitoring should stop temporarily. Let the agent complete
the standing task only after the declared goal is actually terminal. See
[Scheduled tasks](09-scheduled-tasks.md) for overlap, ownership, and server
guardrails.

## Give an agent a signed-in website

Use the Agent Browser when the task needs a real login, consent screen, anti-bot checkpoint, or visual interaction that the project preview cannot provide.

1. Open **Open Browser**.
2. Toggle the key-shaped **Agent browser** control.
3. Wait for the live browser pane.
4. Sign in yourself.
5. Return to the composer.
6. Select the `browser` skill for Claude or Codex.
7. Describe the permitted website task and the stopping condition.
8. Watch the shared browser and intervene when needed.
9. Use the square **Stop the agent browser** control when finished.

The agent and human share one browser profile and one window. Never sign in to an account whose authority you are unwilling to expose to the agent.

## Use a secret-dependent service

1. Open the project gear and select **Secrets**.
2. Add the required key and value.
3. Restart the application process that needs it.
4. Ask the agent to verify only that the variable exists or that the service connects; do not ask it to echo the value.
5. Use `/workspace/.env` only through a library that deliberately loads dotenv files.
6. Delete or rotate the secret when the integration is no longer needed.

Project secrets are readable by every project member and every agent run. They are not an approval boundary.

## Make a risky change with a recovery point

1. Open **History** and check whether the repository is clean.
2. If the tree is dirty, ask the agent to make a meaningful commit or create one in **Terminal**.
3. Fork the chat if you want to preserve the original conversational branch.
4. Run the experiment.
5. Inspect the diff and application result.
6. If you need the old code, select the earlier commit and choose **Switch**.
7. If new dirty changes block **Switch**, commit or stash them in **Terminal**, refresh **History**, and try again.

Git protects tracked project files. It does not back up chats, secrets, users, browser sessions, or untracked external state.

The backend supports an automatic safety-checkpoint checkout, but the current History drawer does not render the checkpoint form needed to complete that path. Use Terminal for dirty-tree recovery until the UI is finished.

## Collaborate with another person

Administrator:

1. Configure Google OAuth in **Settings → Users**.
2. Add the person's email as **member** or **admin**.

Project member:

1. Open the project gear.
2. Select **Sharing**.
3. Add the registered email.
4. Tell the person which project to open.

The new member gains the project's terminal, chats, secrets, uploads, preview, browser, and membership controls. Do not share a project as a read-only review link; Remote currently has no read-only project role or anonymous preview.

## Recover an unresponsive project

1. Open the project gear.
2. Check **Info** for state, memory, process, disk, and network symptoms.
3. If networking is missing, choose **Repair network**.
4. If internal processes are wedged, open **Settings** and choose **Force restart**.
5. If the project is stopped or missing, choose **Start project**.
6. If resource limits caused the problem, ask an admin to raise the relevant limit.
7. Reopen the chat and verify files, agent authentication, IDE, and preview.

Force restart is driven by the host, so it can recover a container that cannot respond from inside. A backend restart loses the run lock, cancellation handle, and event-stream ownership. Because the production unit uses `KillMode=process`, an `lxc exec` child can remain alive but orphaned; the control plane cannot reattach to it.

## Download a deliverable

1. Ask the agent to put the final artifact in `/workspace`.
2. Choose **Files**.
3. Expand folders or use filename search.
4. Hover the file and choose download.
5. For a directory, choose the folder download action to receive a ZIP.

Folder archives are limited to 1 GiB and two concurrent downloads server-wide. For larger artifacts, use the terminal, Git, or an external object store.

## Keep the laptop lightweight

1. Run provider CLIs, builds, IDE, browser, and project processes inside the Remote project.
2. Keep only the Remote control surface open locally.
3. Stop projects when they no longer need active processes.
4. Use project limits to prevent one workload from taking the server.
5. Watch host capacity under **Settings → Info**.

Remote relocates compute; it does not eliminate it. See the measured snapshot and methodology in [Philosophy](../01-overview/00-philosophy.md).
