---
name: code-refactorer
description: "An opinionated skill that is used to restructure existing, working frontend or backend code into clear layers with accurate SOLID while keeping behavior identical. Use when asked to refactor, clean up, decouple, split a god class or component, remove duplication, introduce dependency injection, or reorganize code. This is for behavior-preserving restructuring, not feature work, bug fixing, rewrites, or broad test-suite creation."
---

# Code Refactorer

## Mission

- Refactor code that already exists and works. 
- Improve its structure aggressively while preserving every observable behavior: outputs, side effects, errors, ordering, and timing. 
- A structural change that leaves the requested smell in place is incomplete; a change in behavior is a rewrite, not a refactor.
- Stay inside the code the user asked to restructure. 
- If you discover a bug or missing feature, report it without folding the fix into the refactor.
- If a correctness change is unavoidable, stop and make the behavior change an explicit, separately authorized task.

## References

**Do not load the unrelated reference by default.**

Determine the changed deployment unit before diagnosing or editing, then read the applicable reference completely:

- Folder names in the references are examples; responsibility ownership and dependency direction are the constraints.
- For server processes, workers, persistence, controllers, services, integrations, or other backend code, read [references/backend.md](references/backend.md).
- For browser or client UI, components, hooks or equivalent presentation adapters, frontend services, stores, context, API adapters, or client-side transport, read [references/frontend.md](references/frontend.md).
- For a full-stack change, shared API contract, or move across the client/server seam, read both references.
- For constants, catalogs, feature flags, environment values, or runtime settings, also read [references/configuration.md](references/configuration.md).
- For public data definitions, DTOs, read models, storage records, or behavioral dependency contracts, also read [references/modeling.md](references/modeling.md).
- For a library or tool that is neither frontend nor backend, use the shared rules below and the repository's closest valid local architecture. Read a reference only when its boundary model actually applies.


For repositories containing both deployment units, keep them independently buildable and let them meet only through an explicit API contract. The frontend must not import backend services, repositories, database rows, or provider SDK types. Validate and map boundary data so internal churn stays on its owning side.

## Behavior is frozen

- Before moving code, name the observable behavior being preserved and how the existing compiler, tests, build, or focused runtime check proves it. 
- Re-run the narrowest meaningful check after each structural move and run the repository's full relevant validation before finishing.
- Never smuggle a bug fix into a refactor. 
- *Preserve bugs that are part of current behavior unless the user separately authorizes fixing them.

## Understand the current design

Read before writing, but read to act:

- Resolve the exact requested scope and the callers, contracts, and flows it touches.
- Read applicable repository instructions and architecture documentation.
- Read the target files, controlling interfaces and types, and the closest valid sibling in full.
- Find existing tests and build/type-check commands that cover the behavior.
- Map each responsibility to its present owner and its intended owner in the selected reference.
- Prefer an established valid local boundary over imposing generic folder names. 
- If the local pattern is itself the requested smell, explain why before replacing it.
- Inspect Git status and preserve unrelated user changes. 
- Never sweep unrelated cleanup into the refactor.

Diagnose concretely: name each smell with `file:line`, the responsibility or principle it violates, and the smallest structural move that resolves it.

## Shared design pressure tests

### Clarity and ownership

- A unit should have one coherent owner and reason to change. Split policy, presentation, persistence, transport, and lifecycle when different actors drive them.
- Keep public contracts small and intentional. Do not widen an API merely to make a move convenient.
- Make invalid states difficult or impossible to construct with precise types and invariant-preserving operations.
- Keep I/O, DOM, network, storage, environment, and time at explicit boundaries. Keep pure decisions free of those details.

### Naming

- Name intent, domain meaning, cost, and effects: `activeUsers`, `fetchOrders`, `commit`, `timeoutMs`.
- Use verbs for actions, nouns for values, positive predicates for booleans, and one domain term per concept.
- Remove vague suffixes such as `Manager`, `Helper`, `Util`, `Processor`, and `Impl` when a precise role exists.
- If a cohesive unit cannot be named cleanly, it probably owns multiple responsibilities.
- Use a type-aware rename so references move together.

### Lean design

- Reuse or extend an existing pattern only when it makes the total system smaller and clearer.
- Do not add speculative flags, hooks, interfaces, registries, layers, or compatibility shims.
- Delete pass-through wrappers that add no semantic boundary.
- A little repeated syntax can be cheaper than an abstraction; repeated policy, lifecycle, or branching signals a missing owner.
- Remove dead code, obsolete comments, unused parameters, and temporary delegation once the safe move is complete.

### Types and encapsulation

- Prefer exact types, discriminated states, and validated boundary values over `any`, loose bags, broad strings, casts, or compiler suppressions.
- Put an invariant and the operations that protect it under one semantic owner with a small public surface.
- Shared mutable state usually deserves a cohesive object, class, or store rather than free functions that let callers maintain invariants manually. A hook or equivalent presentation adapter may expose that owner to UI code, but should not become a second state owner. Follow the language and framework's established idiom; do not force classes into a functional UI or free functions into an object-oriented domain model.
- Keep pure stateless transforms simple. Export only what callers genuinely support.

### SOLID, accurately

Use SOLID as a diagnostic, not permission to add abstraction:

- **Single Responsibility:** one actor or reason to change, not one method or arbitrary file size.
- **Open/Closed:** prefer a strategy, registry, or polymorphic contract only for a demonstrated family of variants that otherwise requires repeated central edits.
- **Liskov Substitution:** every implementation must honor the full contract without caller type checks, no-op methods, or unsupported branches.
- **Interface Segregation:** expose role-specific contracts so callers and implementations do not depend on capabilities they do not use.
- **Dependency Inversion:** policy and details meet at the smallest justified abstraction; construct the detail at a composition root.

An interface earns its place through a real policy/detail boundary, supported variant, or useful test seam. YAGNI outranks speculative SOLID.

## Structural smells to hunt

- Import-time startup of sockets, timers, connections, or registration. Move lifecycle behind explicit construction and `start`/`stop` or disposal.
- A god unit combining lifecycle, mapping, policy, rendering, persistence, and I/O. Extract collaborators along ownership boundaries.
- Duplicated domain decisions or transforms. Establish one semantic owner rather than another generic utility bucket.
- Missing teardown for timers, subscriptions, sockets, or other resources.
- Inconsistent boundary error mapping or invalid input silently discarded.
- Concrete infrastructure, protocol, framework, or provider types leaking into policy contracts.
- Application constants or runtime settings defined outside the centralized configuration owner.
- Defensive branches for impossible internal states. Fix the model or owner instead.

## Make mechanically safe moves

- **Extract:** copy the responsibility to its new owner, make the old site delegate, verify, migrate callers, then remove the old body.
- **Change a contract:** add the new shape alongside the old, migrate callers in bounded steps, and delete the old shape only when unreferenced.
- **Introduce a seam:** derive the smallest contract from actual consumer usage, make the concrete satisfy it, inject it, and wire it at the composition root.
- **Replace conditional dispatch:** reproduce the current branches behind the new dispatch, move one branch at a time, and remove the old conditional only when empty.
- **Move or rename:** use language-aware tooling when available; let the compiler identify affected references and fix every result.

Keep moves small and verified. Always commit your work so you can keep track of the changes history. Follow [references/workflow/commits.md](references/workflow/commits.md); otherwise leave changes uncommitted.

## Verify behavior

- Type-check or compile after each move in typed code.
- Run existing lint, build, and tests using the repository's own commands; use the narrowest relevant check first.
- Add a small characterization test only when moving untyped or uncovered logic whose output could silently drift. 
- Pin current behavior, bugs included; do not build a new suite.
- Inspect risks tests often miss: ordering, error type and text, logs, `null` versus `undefined`, precision, async sequencing, resource lifecycle, and side-effect count.
- If a check cannot run, state exactly which proof is missing. Never imply that passing compilation alone proves all runtime behavior.

## Finish cleanly

Review the final diff as the next maintainer:

- Does each changed responsibility have one obvious owner in the selected frontend or backend architecture?
- Did dependencies move in the intended direction without leaking concrete details?
- Is the result smaller or clearer, with no temporary adapters, dead code, or broadened public surface left behind?
- Did every caller move, and did relevant verification pass?
- Are unrelated files and pre-existing user changes untouched?

Report:

1. **Smell → principle → fix**, with `file:line` evidence.
2. **Behavior preserved by**, naming the checks actually run.
3. **Deliberately not done**, including out-of-scope smells, unverified risks, and any behavior changes that need separate authorization.