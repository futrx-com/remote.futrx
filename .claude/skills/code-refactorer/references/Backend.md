# Backend refactoring reference

Read this reference completely for backend, server, worker, persistence, integration, or request-delivery refactors. Preserve a repository's valid existing architecture; adapt the names below while keeping responsibility ownership and dependency direction explicit.

## Target ownership

Default to grouping by layer or role. When a layer becomes large, subdivide inside it by capability, such as `services/payment/` or `clients/market-data/`. A genuinely feature-oriented codebase may group by feature when each feature owns a cohesive vertical slice; do not reorganize it merely to match generic folder names.

Each unit should answer one primary question:

- **`controllers/` — inbound delivery.** Parse and validate request inputs, invoke one application operation, and map its result to the delivery protocol. Keep business rules and persistence out.
- **`services/` — policy and workflows.** Own business decisions, orchestration, transactions, and domain transitions. Depend on contracts for persistence and integrations, not on SQL, HTTP, SDKs, or global singletons.
- **`repositories/` — owned persistence.** Read and write the application's database, files, or durable state. Hide SQL, ORM, storage schemas, locking, and persistence mechanics behind a consumer-relevant contract.
- **`clients/` — external integrations.** Translate calls to third-party services and provider protocols behind a stable application-facing contract. Translate provider data and errors before they escape.
- **`model/` — meaningful data definitions.** Organize domain, application, API, and persistence models by context as defined in [modeling.md](modeling.md).
- **`port/` — behavioral contracts.** Keep inbound and outbound dependency capabilities separate from data definitions as defined in [modeling.md](modeling.md).
- **`transport/` — wire mechanics.** Own HTTP, WebSocket, RPC, gRPC, SSE, serialization, connection lifecycle, retries, reconnects, and protocol-level authentication. It knows how bytes move, not what the business decides.
- **`middleware/` — cross-cutting delivery pipeline.** Own concerns such as request authentication, correlation, logging, and protocol-level error mapping that consistently wrap many controllers.
- **`config/` — constants and runtime settings.** Centralize constants, catalogs, defaults, flags, and validated environment values as defined in [configuration.md](configuration.md). Keep dependency construction in the entrypoint or composition root.
- **`shared/` — dependency-light backend utilities.** Hold genuinely reusable pure helpers and foundational result/error types. It must not become a route around service or domain ownership.

Skip folders that have no real responsibility yet. A small service can remain a flat controller/service/repository trio; an extra layer must earn its existence.

## Dependency direction

The runtime flow is normally:

```text
controller -> service -> repository contract
                      -> client contract
```

Concrete repositories and clients satisfy consumer-owned contracts; the entrypoint wires them at the composition root using typed configuration. Evaluate runtime ownership and concrete leakage, not imports in isolation: an adapter may import a service-owned port in order to implement it.

Infrastructure must not leak upward:

- no SQL, ORM model, transaction handle, or storage exception in a service contract;
- no provider SDK object or response type escaping a client;
- no HTTP request, status code, header, or socket detail inside business policy;
- no service constructing its concrete repository, provider client, clock, or global singleton.

Controllers, repositories, and clients are boundary adapters in different directions:

- a controller hides how requests arrive;
- a repository hides where owned data lives;
- a client hides which external provider is called;
- transport hides how bytes cross the process boundary.

## Placement questions

- Does the outside world call this request handler? Put delivery mapping in a controller.
- Is it a business decision or workflow such as "validate, charge, then persist"? Put it in a service or existing application-policy owner.
- Does it read or write owned durable state? Put it in a repository or established store adapter.
- Does it call someone else's service? Put provider translation in a client or integration adapter.
- Is it protocol, serialization, retry, or connection machinery? Put it in transport.
- Is it a constant, catalog, flag, default, or environment value? Put it in config.
- Is it dependency construction, registration, or startup? Put it at the entrypoint/composition root.
- Is it a data definition or behavioral contract? Classify it using [modeling.md](modeling.md).
- Is it pure and broadly reused? Put it in a dependency-light shared module only when no semantic owner is better.

If one file answers several of these questions, split it along those boundaries.

## Example structure

```text
backend/
  controllers/
  services/
    payment/
    parsing/
    market-data/
  repositories/
  clients/
  model/
  port/
  transport/
  middleware/
  config/
  shared/
```

This is one realization, not a mandate. A recursive listing should reveal the backend's real layers and capabilities before a maintainer opens a file.

## Backend-specific pressure tests

- Keep transaction boundaries with the workflow that requires atomicity; do not let unrelated controllers or repositories coordinate a business transaction.
- Make startup and shutdown explicit. Importing a package must not open listeners, start goroutines/tasks, or connect to infrastructure.
- Keep retry policy at the owner that understands idempotency. Transport may implement retry mechanics, but business policy decides whether an operation is safe to repeat.
- Validate untrusted input at delivery and integration boundaries. Internal policy should receive a trusted, precise type rather than repeatedly defending against malformed wire values.
- Map boundary errors once and consistently. Preserve existing public status codes, response bodies, error types, transaction behavior, log events, and retry timing during a refactor.
- Avoid pass-through services or repositories that exist only to satisfy a diagram. Every adapter or orchestration layer must hide a real detail or own a real decision.

## Backend verification

In addition to the shared checks in `SKILL.md`, verify the relevant existing behavior around:

- response status, body, headers, and error mapping;
- persistence writes, query semantics, ordering, locking, and transaction commit/rollback;
- external request shape, provider error translation, retry count, and idempotency;
- startup/shutdown, connection disposal, and background-task lifecycle;
- public contracts and serialization at every changed boundary.