---
name: add-bounded-context
description: Scaffold a new bounded-context module (or make an existing shell module real) following the UMS four-layer template with its own DbContext, engine, and per-module unit of work. Use when creating/implementing a module under src/Modules.
---

# Add / implement a bounded-context module

Use this to create a new module or turn one of the shell modules (Identity, Finance, ...) into a real
one. Mirror `src/Modules/Lms` (fully worked, Postgres + Redis) and `src/Modules/StudentManagement`
(structure). Read `.claude/memory/conventions.md` first.

## 1. Four projects
Create `<Module>.Domain`, `.Application`, `.Infrastructure`, `.Presentation` with the same references
as the Lms projects (copy the `.csproj` files and rename). Namespaces `<Module>.<Layer>`.
- Domain → `BuildingBlocks.Domain` only.
- Application → its Domain + `BuildingBlocks.Application` (+ MediatR, FluentValidation).
- Infrastructure → its Application/Domain/Presentation + BuildingBlocks (+ EF provider packages).
- Presentation → its Application + `BuildingBlocks.Web` (+ MediatR).

## 2. Register it
- `dotnet sln add` the four projects.
- Add a project reference to `<Module>.Infrastructure` in `src/ApiHost/ApiHost.csproj`.
- Add `new <Module>.Infrastructure.<Module>Module()` to `src/ApiHost/ModuleRegistry.cs`.

## 3. The IModule + DI
- `<Module>Module : IModule` with `const string ModuleName`, `RegisterModule` (calls
  `Add<Module>Application()` + `Add<Module>Infrastructure(config)`), and `MapEndpoints`.
- DbContext derives from `ModuleDbContext`, sets its own `Schema`, applies its configurations, and
  registers the domain-events interceptor. Read `ConnectionStrings:<Module>`; choose the engine (use
  Npgsql like Lms if going relational, else InMemory as a fallback).

## 4. Per-module unit of work (MANDATORY — see the gotcha)
- **Do NOT reuse the shared `IUnitOfWork` from handlers.** Define
  `I<Module>UnitOfWork : IUnitOfWork`, implement it on the DbContext, and register
  `AddScoped<I<Module>UnitOfWork>(sp => sp.GetRequiredService<<Module>DbContext>())`. Handlers inject
  the module-specific interface. Details: `.claude/memory/ums-iunitofwork-collision.md`.

## 5. Isolation + tests
- The module must reference **no other module** — add it to the isolation assertions in
  `tests/ArchitectureTests/ModuleIsolationTests.cs`.
- Add an integration test project (copy `tests/Lms.IntegrationTests`) including the swallowing-UoW
  regression guard.
- `dotnet build` (0 warnings) + `dotnet test` (green) before pushing.

Then implement features with the `add-lms-feature` skill's pattern (Domain → Application →
Infrastructure → Presentation → migration → test).
