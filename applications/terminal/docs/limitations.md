# terminal limitations

- **Terminal shells are not durable.** The Terminal application reconnects to the same shell after a dropped connection, but ends a shell that has had no viewer for 10 minutes, and ends every shell when the application is stopped, restarted, or upgraded.

The service listens on a root-only Unix socket; it exposes no TCP port.
