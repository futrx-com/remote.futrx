# Stack: Python

This workspace was provisioned from the `python` project template.

- `python3` is Python 3.12 (Ubuntu 24.04 system interpreter).
- `/workspace/.venv` is a durable project virtualenv. Activate it with
  `source /workspace/.venv/bin/activate`, or call
  `/workspace/.venv/bin/python` directly.
- `uv` is installed at `/usr/local/bin/uv`. Prefer `uv pip install` over bare
  `pip` — it is much faster on a small host.
- `pipx` is available for CLI tools. Poetry is intentionally NOT installed;
  run `pipx install poetry` only if the project actually uses it.

Never `pip install` into the system interpreter: Ubuntu marks it as an
externally managed environment and the install will be refused. Use the venv.

## Previews

Bind servers to `0.0.0.0`, not `127.0.0.1`, or the preview proxy cannot reach
them. Port 8000 is this template's convention. Run long-lived servers as
transient systemd units so they outlive the agent turn:

```bash
systemd-run --unit=dev-server --working-directory=/workspace \
  /workspace/.venv/bin/uvicorn app:app --host 0.0.0.0 --port 8000
```
