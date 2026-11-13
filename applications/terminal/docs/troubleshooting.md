# Terminal troubleshooting

### The chat header has no terminal button

The terminal is the **Terminal** application. It appears only in a project chat
whose project has the application installed and running. Install it from the
project's Applications page, or ask an administrator to install it globally so
every project gets it. If it is installed but stopped, start it there.

### Terminal shows Reconnecting

The Terminal pane reconnects by itself while it is open and returns to the same
shell with its recent output. If it stays on **Reconnecting**, check that the
project and its Terminal application are running, and that your browser can
still reach Remote.

### Terminal came back with a new shell

A shell nobody is viewing is ended after 10 minutes, and stopping, restarting,
or upgrading the Terminal application ends every shell in the project. Run
durable processes under a service manager inside the project.

