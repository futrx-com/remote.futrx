---
name: plan
description: Produce a written implementation plan for a feature, refactor, or bugfix in any codebase before writing code. Use when the user says "plan", "design", "how should we build X", "break this down", or asks for an approach on a non-trivial change — and also whenever a request would otherwise send you straight into editing files across more than one module. Runs a documentation-first research pipeline, discovers and conforms to the architecture already in the repository, applies a SOLID and design-pattern review, and self-verifies the plan before handing it over. Outputs a plan file under docs/plans/ for a downstream implement or QA step to consume. Not for changes with no design decision in them — renames, typo fixes, formatting, dependency version bumps — do those directly.
---

# Planning a change

End the session with a plan file that another agent — or a future you, cold, with no memory of
this conversation — can execute without re-deriving anything. A plan that restates the request is
a failed plan. A plan that invents an architecture the repository does not already use is a worse
one, because someone has to live with it afterwards.

This skill assumes nothing about the language, framework, or layout of the project. Everything it
needs, it discovers. Where you would normally rely on what you already know about a stack, the
pipeline asks you to go and confirm it in the repository instead — because your recollection of a
framework is a snapshot, and the project in front of you is not.

**Budget: thoroughness wins.** Read widely, read fully, re-read when unsure. Do not skip a
document, truncate a file, or shorten a phase to save tokens or time. A plan built on a partial
read costs far more later than the tokens it saved. If you catch yourself thinking "this is
probably how it works" — that is the signal to go and read it.

## Four rules that outrank everything else

1. **Conform before you improve.** When the repository's convention conflicts with what you
   consider best practice, follow the repository and record the gap. Consistency is a feature; a
   codebase with two competing styles is worse than one with a single imperfect style.
2. **Read before you design.** Every design claim traces to something you actually read, with a
   `path:line` anchor. Anything else is a guess wearing a plan's clothing.
3. **Justify structure, never decorate with it.** Every abstraction, pattern, and extension point
   answers a named pressure from the actual requirements. Structure with no force behind it is
   speculative generality, and it ages badly.
4. **Verify the plan before handing it over.** The final phase is not optional and not a
   formality.

## Constraints that hold in every project

- **Never hand-edit generated, vendored, or lock files.** Identify them during orientation and
  plan changes to their *source* instead — the generator input, the schema, the manifest.
- **Never assume the schema-migration policy.** Find the documented command and workflow before
  planning any schema change. Destructive or irreversible operations require an explicit rollback
  story in the plan.
- **Never plan against a framework version you have not confirmed.** Read the version from the
  manifest and lockfile. When it is newer than what you are confident about, read the project's
  vendored or pinned documentation before planning any API, routing, caching, or lifecycle
  decision. A confident answer from memory is the most likely way a plan goes wrong.
- **Never put credentials, tokens, or environment secrets in a plan file.** Reference the config
  key by name.
- **A project's own rules outrank this skill.** If the repository documents a convention that
  contradicts anything here, follow the repository and note the override in the plan.

## Bundled files

- `references/design-review.md` — the full rubric for Phase 5, with worked examples. Read it
  before filling in the Design section.

---

# The pipeline

Run the phases in order. Each phase has a gate: do not enter the next phase until the gate
passes. If a gate cannot pass because the answer requires a human, see **Blocking** below.

## Phase 0 — Frame the ask, and size it

State the request in one sentence, and state what is explicitly out of scope. If two readings of
the request lead to materially different builds, stop and ask now — not after 200 lines of plan.

Then classify the change. Running nine phases on a one-file bugfix is not just wasted effort — it
teaches you to route around this skill for small work, and that habit spreads to work that needed
it.

| Tier | It qualifies when | Run |
| --- | --- | --- |
| **Trivial** | One module, no new abstraction, no schema change, no new dependency, no change to a public interface | Phases 0, 3, 6. Deliver the plan inline; no plan file unless the project requires one |
| **Standard** | Anything that is neither trivial nor structural — the default | All phases |
| **Structural** | Adds or moves a module boundary, changes a public interface or schema in a breaking way, introduces a pattern the repository does not use, or adds a dependency | All phases, plus a **blocking check-in** with the user after Phase 5 and before Phase 7 |

Record the tier and the one fact that decided it, so a wrong call is visible to a reviewer rather
than buried. **When torn between two tiers, take the higher one.** The cost of over-planning a
medium change is an hour; the cost of under-planning a structural one is a refactor.

**Gate:** one-sentence scope, at least one written non-goal, and a recorded tier with its reason.

## Phase 1 — Orientation

Establish what kind of project this is before reading anything in depth. You are building the map
the rest of the pipeline navigates by. Record each finding with the path that evidences it.

Determine:

- **Languages, package manager, and build system** — from manifests and lockfiles.
- **Exact versions of the frameworks this change touches** — from the lockfile, not the manifest
  range. Note which are newer than your confident knowledge; those need local docs.
- **Repository shape** — monorepo or single package, workspace boundaries, where each deployable
  unit lives, and each one's entry point.
- **Where documentation lives** — root docs, per-module docs, ADRs, agent-instruction files
  (`AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, or equivalents), vendored framework docs.
- **How the project is built, run, and checked** — the actual scripts or commands.
- **Test setup** — the runner, where tests live, how they are invoked, and their coverage of the
  area you are touching. If there is no test infrastructure, record that explicitly: it changes
  how every step in this plan gets verified.
- **Data layer and migration tooling** — the ORM or query layer, where schema and migrations
  live, and the documented migration workflow.
- **Generated, vendored, or committed-artifact directories** — anything a human should never edit
  by hand.
- **Lint, format, and type-check configuration** — these encode conventions you must follow.

**Gate:** every item answered, or explicitly recorded as absent from the project.

## Phase 2 — Documentation sweep

Read the project's documentation before any design thinking, and read all of it that could bear on
this change. Docs encode decisions whose reasoning is invisible in the code; skipping them is how
an agent confidently re-litigates a settled question.

Cover, at minimum, whichever of these exist:

- Root `README`, agent-instruction files, and `CONTRIBUTING`
- Architecture notes and ADRs — decision records are the highest-value reading in most
  repositories, because they tell you what was already rejected and why
- Documentation inside the module(s) this change lands in
- Domain or product documentation that defines the terms the request uses
- Vendored or pinned framework documentation for the specific version, for every framework
  behaviour this change depends on
- **Previous plans** in `docs/plans/` (or wherever this project keeps them) for the same area —
  and in particular their `## Outcome` sections, which record what went wrong last time. A plan
  that repeats a documented failure is the most avoidable kind of bad plan.
- Any file stating hard project constraints — migration policy, deployment policy, security or
  compliance rules, forbidden dependencies

Keep a **sources ledger** as you go: file path, and one line on what it settled. This ledger goes
into the plan file verbatim — it is how a reviewer knows the plan rests on something.

Lift every hard constraint you find into a **Hard constraints** section of the plan, quoted with
its source. The plan must not contradict them, and a reviewer should not have to go looking.

**Gate:** ledger has an entry for every document category that exists in the repository, or an
explicit note that it does not.

## Phase 3 — Code and structure reconnaissance

Documentation tells you intent. Code tells you what is actually true. Escalate to code whenever
the docs leave you short — and apply this test honestly, because the failure mode here is an agent
persuading itself that it already understands.

Answer all five questions, each with a concrete `path:line` anchor. **If any answer is a
paraphrase of the docs rather than a pointer to code, you have not finished this phase.**

1. Which module and which layer does this change belong to, and where are that layer's boundaries
   — what is it allowed to depend on, and what depends on it?
2. What is the closest existing feature that is structurally similar to this one? Name it. This is
   your **reference implementation**, and the new work should read like a sibling of it.
3. What are the naming, file-layout, and export conventions in that area?
4. How does that area handle errors, validation, logging, authorization, configuration, and
   transactions?
5. What is the end-to-end flow of the reference implementation — entry point → business logic →
   persistence or external call → response shape?

Grep for the pattern the change should imitate rather than reading files at random. When the
change touches persisted data, read the actual schema definitions and the most recent migrations,
not only the migration documentation.

**Gate:** all five answered with anchors, and a named reference implementation.

## Phase 4 — Architecture conformance

Before designing anything new, write down what you are conforming to. Build a conformance ledger:

| Convention observed | Evidence (`path:line`) | How this change follows it |
| --- | --- | --- |

Cover layering and module boundaries, dependency direction, naming, error and result shapes,
validation placement, data-transfer versus domain types, and how modules expose themselves to each
other.

If the change genuinely cannot follow an existing convention, that is a **deviation**, not a free
choice. Record it as: what convention is being broken, why it cannot be followed, what the
resulting inconsistency costs, and how it would be reconciled later. Deviations are allowed;
silent deviations are not.

**Gate:** conformance ledger written; every deviation has all four fields.

## Phase 5 — Design

**Read `references/design-review.md` before starting.** It carries the rules and worked examples
for the three tables below; the summaries here are not sufficient on their own.

Work through 5a–5e in order and record each — they all appear in the plan file.

- **5a. Forces.** List the pressures the design must answer, drawn from the actual requirements —
  a variation known to be coming, a second consumer, an external system that can fail, permission
  rules that differ by role, data volume, tenancy, auditability, backwards compatibility, data
  migration. Every abstraction introduced later traces back to a force on this list. If a piece of
  structure traces to nothing, delete it.
- **5b. Patterns.** One row per design concern: concern, pattern chosen, force it answers, prior
  art in the repo, rejected alternative with a one-line reason. A pattern the repository already
  uses beats a "better" one it does not. "No pattern — plain module, reason: …" is a correct row.
- **5c. SOLID review.** One line per principle, each pointing at a concrete element of *this*
  design. Restating the definition is not a review.
- **5d. Extension and override points.** One row each: the point, the mechanism, the named
  variation it anticipates, and why that variation is plausible *now*. Rows justified by a
  hypothetical get deleted.
- **5e. Maintainability.** Placement matches convention; no duplicated utility; every new
  dependency justified; name the code this makes dead; naming reads consistently beside the
  reference implementation.

**Gate:** 5a–5e recorded, every abstraction traces to a force. **If the tier is Structural, stop
here and check in with the user before writing the plan file** — present the approach, the pattern
choices, and the deviations, and get agreement. A structural decision is cheapest to change at
this exact moment.

## Phase 6 — Decompose into verifiable steps

Break the work into steps that are independently verifiable. Each step states what changes, in
which files, and how you will know it worked — a command to run, or an observable result. A step
you cannot verify is a step you should split until you can.

Match verification to what the project actually has. With a test runner, name the test and the
command. Without one, name the observable: a request and its expected response, a log line, a
query result, a screen state. "It should work" is not a verification.

Order steps so the repository is in a working state between them wherever possible. Schema and
data-migration steps carry their ordering constraints explicitly.

**Gate:** every step has a verification; no step is "and then it works".

## Phase 7 — Write the plan file

Write to `docs/plans/<YYYY-MM-DD>-<kebab-slug>.md` — or wherever this project already keeps plans,
if that differs. Use the template below, then stop. **Planning does not write implementation
code.**

## Phase 8 — Verification gate

Auditing your own plan by re-reading it is weak: you read your intent rather than the text on the
page. So this phase runs two passes, mechanical before judgmental.

### 8a — Mechanical checks

These need no judgment, which is exactly why they come first — they cannot be rationalised away.
Scan the written file and confirm:

- [ ] The Context section contains at least one `path:line` anchor per module touched. Zero
      anchors anywhere is an automatic fail
- [ ] Every numbered step contains a `Verify:`
- [ ] Every table row has no empty cells
- [ ] Every pattern row has a non-empty force, plus either prior art or a rejected alternative
- [ ] No extension-point justification contains "might", "in case", "later", or "future-proof"
- [ ] Data / migrations section is non-empty — real content or the literal word "None"
- [ ] Hard constraints section is non-empty — real constraints or "None found"
- [ ] Tier is recorded, and the phases actually run match that tier

### 8b — Cold-reader pass

Now test the claim that a stranger could execute this.

**If you can spawn a subagent:** give it the plan file path and nothing else — no conversation
history, no context from this session. Ask it to list every point where it would have to guess or
come back with a question. Every item it returns is a defect in the plan.

**If you cannot:** read the file top to bottom without referring back to this conversation, and
write that guess-list yourself. Do not consult the conversation while doing it — the conversation
is precisely the context the reader will not have.

Then check the judgment items:

- [ ] Scope is one sentence, and non-goals are explicit
- [ ] Orientation findings recorded, including framework versions read from the lockfile
- [ ] Sources ledger covers every document category present in the repository
- [ ] Nothing in the plan contradicts the hard constraints
- [ ] All five Phase 3 questions answered with anchors; a reference implementation is named
- [ ] Conformance ledger present; every deviation has all four fields
- [ ] Every abstraction traces to a force in 5a
- [ ] SOLID review points at concrete elements of this design, not at definitions
- [ ] Every step's verification uses tooling the project actually has
- [ ] Risks have mitigations; open questions marked blocking or non-blocking
- [ ] Verification plan gives the downstream QA step checkable acceptance criteria
- [ ] The cold-reader guess-list is empty, or every item on it has been fixed

**If any item fails: fix the plan, then run 8a and 8b again.** Record the outcome in the plan's
Verification gate section, including what the cold reader flagged and what changed as a result.

**Gate:** both passes clean, and the result written into the file.

## Blocking

If a gate cannot pass because the answer requires a human decision, do not guess and do not stall
silently. Write the plan file up to that point, set `Status: blocked`, put the question at the top
of **Risks & open questions** marked `BLOCKING`, and tell the user exactly what you need. A partial
plan with a sharp question is useful; a confident plan built on a guessed premise is not.

---

# Output template

Each section is the recorded output of the phase named beside it. Fill every section or mark it
explicitly "None".

```markdown
# <Title>

**Status:** planned | blocked | in-progress | done
**Tier:** trivial | standard | structural — <the one fact that decided it>
**Scope:** <one sentence>
**Out of scope:** <explicit non-goals>

## Orientation                              <!-- Phase 1 -->
Stack and versions (from lockfile), repo shape, build/run commands, test setup or its absence,
migration tooling and workflow, generated paths that must not be hand-edited.

## Hard constraints (this project)          <!-- Phase 2 -->
- <constraint> — source: `path`

## Sources read                             <!-- Phase 2 -->
| Document | What it settled |
| --- | --- |

## Context                                  <!-- Phase 3 -->
What exists today, with `file:line` anchors.
**Reference implementation:** `path` — the existing feature this change should read like.
**Flow today:** entry → logic → persistence/external → response.

## Architecture conformance                 <!-- Phase 4 -->
| Convention observed | Evidence | How this change follows it |
| --- | --- | --- |

**Deviations:** <convention> — <why unavoidable> — <cost> — <how it gets reconciled>. Or "None".

## Design                                   <!-- Phase 5 -->
### Forces
- <force> — <where it comes from in the requirements>

### Approach
The chosen design in a short paragraph.

### Patterns
| Concern | Pattern | Force | Prior art | Rejected alternative — reason |
| --- | --- | --- | --- | --- |

### SOLID review
- **S:** … **O:** … **L:** … **I:** … **D:** …

### Extension points
| Extension point | Mechanism | Variation anticipated | Why plausible now |
| --- | --- | --- | --- |

### Maintainability
Placement, reuse, dependencies, code to delete, naming consistency.

## Steps                                    <!-- Phase 6 -->
1. **<Step name>** — files: `a`, `b`. Change: <what>. Verify: <command or observable>.

## Data / migrations                        <!-- Phase 6 -->
Migration name, forward summary, rollback plan, regeneration or reseed required, ordering
constraints. "None" if none.

## Risks & open questions
- <risk> → <mitigation>
- [ ] `BLOCKING` <question needing a human answer>

## Verification plan
Acceptance criteria as checkable statements for the QA step.

## Verification gate                        <!-- Phase 8 -->
Mechanical pass: clean / fixed <what>.
Cold reader flagged: <list, or "nothing">. Resolved by: <what changed>.

## Outcome                                  <!-- left empty; filled after implementation -->
What was wrong, missing, or redesigned mid-flight. What the next plan in this area should know.
```

The `## Outcome` section is written by whoever implements the plan, not by you. Leave it in place
and empty. Phase 2 reads it on the next plan in the same area, which is how this pipeline learns
from real failures instead of repeating them.

# Done when

Both verification passes are clean, the plan file exists, and every section is filled in or
explicitly marked "None". Tell the user the plan path, the tier, the reference implementation the
design follows, and the one decision you most want them to confirm before implementation starts.