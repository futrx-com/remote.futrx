# Terminal usage

## Run a terminal command

Install the **Terminal** application before opening a shell. An administrator
can install it for every project from the global Applications page, or a
project member can install it for their project. The terminal button appears
in the chat header once the application is installed and running.

1. Select **Container terminal** in a project chat's header.
2. Wait until the pane's status reads **Connected**.
3. Check the path shown beside the status before running a command. It is the
   path inside the container, for example `/workspace`.
4. Run the command in the interactive login shell.
5. Select **Close Terminal** when finished.

**Outcome:** Terminal runs `bash -l` as root inside the project container.
It starts in the chat's working directory if that directory is inside
`/workspace`. Otherwise, it starts in `/workspace`.

On desktop, Terminal opens as a pane beside the chat. Drag its left edge to
resize it; Remote remembers that width in this browser. Opening Files, History,
or Browser hides Terminal. Only one workspace pane can be open at a time.

The status reads **Connecting**, **Connected**, **Reconnecting**, or
**Closed**.

Each chat has its own shell. You can return to it after a dropped connection
or a page reload:

- If the connection drops while the pane is open, Terminal reconnects by itself
  and returns to the same shell, with its recent output replayed.
- Closing and reopening the pane, reloading the page, or switching chats and
  coming back also returns to the same shell.
- A shell ends after 10 minutes without a viewer. Stopping, restarting,
  or upgrading the Terminal application ends every shell in that project.
- When the shell exits, the status reads **Closed**. Press **Enter** in the
  terminal to start a new one.
- Press **Ctrl+F** (**Cmd+F** on macOS) in the terminal to search its output.

The terminal does not start a stopped project or a stopped Terminal
application; start it from the project's Applications page.

Use a service manager inside the project for commands that need to keep
running unattended. Terminal reconnects to shells, but does not keep them
running indefinitely.

