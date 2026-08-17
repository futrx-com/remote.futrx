# Stack: Node.js

This workspace was provisioned from the `node` project template.

- Node 22, `npm`, `npx` are on `PATH` from the base image.
- `pnpm` and `yarn` are enabled through `corepack`; prefer `pnpm`.
- Nothing else is installed. `apt-get install` whatever the project needs.

## Previews

Dev servers must bind `0.0.0.0` (not `127.0.0.1`) to be reachable through the
platform's preview URLs. Ports 3000 and 5173 are the conventional ones for
this template, but any listening port is discovered automatically.

Run long-lived dev servers as transient systemd units so they survive the
agent turn that started them:

```bash
systemd-run --unit=dev-server --working-directory=/workspace/app \
  pnpm dev --host 0.0.0.0 --port 5173
```
