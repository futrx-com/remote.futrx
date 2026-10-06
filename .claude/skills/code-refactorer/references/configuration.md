# Configuration

Read when changing constants, catalogs, defaults, flags, environment values, or runtime settings.

```text
config/
  constants/
    pagination.*
    agent-models.*
  runtime/
    schema.*
    load.*
```

- Put all application constants and canonical catalogs in `config/constants/`, grouped by domain rather than one giant file.
- Search this registry before adding a constant. Reuse the canonical definition; do not create near-duplicates.
- Keep distinct meanings distinct even when values match, such as `DEFAULT_PAGE_SIZE` and `MAX_PAGE_SIZE`.
- Put environment loading, validation, defaults, and typed immutable settings in `config/runtime/`.
- `config/` contains values only: no business decisions, I/O workflows, mutable state, or dependency construction.
- Construct dependencies in the backend entrypoint or frontend `app/` composition root.
- Backend secrets may be read from the environment but never hardcoded. Frontend configuration must be public.
- A cross-stack constant has one canonical owner and is shared through an API contract or shared package, never copied.

Examples: `LIMIT_PER_PAGE` belongs in `config/constants/pagination.*`; agent model definitions belong together in `config/constants/agent-models.*`.