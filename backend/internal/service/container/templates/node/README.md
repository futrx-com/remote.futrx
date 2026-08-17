# Node.js workspace

This project was created from the **Node.js** template.

- Node 22, `npm`, and `npx` come from the base image.
- `pnpm` and `yarn` are enabled through `corepack`.

## Start a project

```bash
cd /workspace
pnpm create vite@latest app
cd app && pnpm install
pnpm dev --host 0.0.0.0 --port 5173
```

Bind dev servers to `0.0.0.0`. A server on `127.0.0.1` is invisible to the
preview proxy, which reaches the container over its network address.
