# Data modeling

Read when moving or creating public data definitions, DTOs, commands, queries, read models, storage records, or dependency interfaces and traits.

## Canonical organization

Use singular `model/` and `port/` roots across languages:

```text
model/
  <context>/
    domain/
    application/
    api/
    persistence/
port/
  <context>/
    inbound/
    outbound/
```

- `domain/`: entities, value objects, aggregates, and domain events.
- `application/`: commands, queries, use-case results, and read models.
- `api/`: request, response, event, and message DTOs at process boundaries.
- `persistence/`: database rows, documents, and ORM records.
- `port/`: behavioral capabilities; inbound ports expose application operations and outbound ports describe required dependencies.

Use `model/`, not `data/`, `shared/`, `types/`, or `interfaces/`, as the modeling root. `<context>` prevents it from becoming a global dumping ground. Map between categories at their boundaries; domain models must not depend on API or persistence models.

## Classify by meaning

- A data definition belongs in the appropriate `model/` category even when its language construct is named `interface`.
- A behavioral interface or trait belongs in `port/` only when it exposes an application operation or required dependency across a boundary. Keep local strategies and generic constraints with their owner; a domain model may still own invariant-preserving methods.
- Keep private implementation-only buffers, projections, mapper state, and helper types beside their implementation.
- Do not move services, controllers, repositories, clients, stores, or framework components into `model/` merely because they are structs or classes.

## Language mapping

| Language | Physical roots | Models | Ports |
| --- | --- | --- | --- |
| Go | `internal/model/`, `internal/port/` | structs, enums expressed as named types | consumer-owned interfaces |
| TypeScript | `src/model/`, `src/port/` | types, data-only interfaces, classes, enums | dependency-capability interfaces |
| Rust | `src/model/`, `src/port/` | structs and enums | boundary capability traits |
| Java | `<base>.model`, `<base>.port` | records, POJOs, and enums | boundary capability interfaces |

In Go, treat `model/` and `port/` as grouping directories, not single catch-all packages; make leaf packages context-specific. Apply the same bounded-context grouping in the other languages.