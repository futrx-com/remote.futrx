---
name: hello-remote-inspector
description: Inspect and verify the Hello Remote example application's container service and runtime metadata. Use when a user asks to test, diagnose, or demonstrate an installed Hello Remote application.
---

# Hello Remote Inspector

Verify the installed example from inside its project container.

1. Run `systemctl is-active hello-remote` and report the exact state.
2. Run `/usr/local/bin/hello-remote-info` to read safe container facts.
3. Run `/usr/local/bin/hello-remote-service health --port 4780` to test the declared internal port.
4. When response details matter, request `http://127.0.0.1:4780/health` and parse its JSON.
5. If a check fails, inspect `systemctl status hello-remote --no-pager --full` and `journalctl -u hello-remote -n 100 --no-pager` before suggesting a fix.
