# Python workspace

This project was created from the **Python** template.

- System interpreter: Python 3.12 (`python3`).
- Project virtualenv: `/workspace/.venv` — durable, survives container
  replacement.
- Package managers: `uv` (preferred) and `pip`.

## Everyday use

```bash
source /workspace/.venv/bin/activate
uv pip install fastapi uvicorn
uvicorn app:app --host 0.0.0.0 --port 8000
```

Poetry is not installed. Add it with `pipx install poetry` if the project
needs it.
