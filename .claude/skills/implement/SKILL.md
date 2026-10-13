---
name: implement
description: Execute a written implementation plan from docs/plans/ — writing the actual code, one verified step at a time. Use when the user says "implement", "build it", "execute the plan", "do it", "continue the implementation", or points at a plan file and asks for it to be carried out. Also use when resuming a partly-finished implementation. Reads the plan in full before touching code, scales its process to the tier the plan recorded, re-checks the plan's assumptions against the current repository, verifies each step before starting the next, stops rather than redesigning when the plan turns out to be wrong, and writes the outcome back into the plan file for the next planning cycle. Not for exploratory coding with no plan — write a plan first with the `plan` skill.
---

# Implementing a plan

The plan already made the decisions. Your job is to carry them out faithfully, to notice
immediately when reality disagrees with the plan, and to leave behind a repository that works and
a record of what actually happened.

The two ways this goes wrong are opposites, and both are common. One is drifting — quietly
improving the design as you go until the code no longer matches the plan anyone reviewed. The
other is stubbornness — following a step that has become impossible because the ground moved,
producing code that satisfies the document and nothing else. The pipeline below is built to keep
you between the two.

**Budget: correctness over speed.** Verify every step for real. If you are unsure whether
something worked, it did not.

## Four rules that outrank everything else

1. **The plan is the scope.** You implement the plan and nothing else. Anything else you notice
   goes on a list for the user, not into this diff. An implementation that also fixed three
   unrelated things is an implementation nobody can review.
2. **One step at a time, verified before the next.** Never write step 4 while step 2 is
   unverified. Batching feels faster and reliably costs more, because when something breaks you
   have lost the information about which change broke it.
3. **Never edit a verification to make it pass.** Changing a test, loosening an assertion, or
   swapping the acceptance criterion for an easier one converts a real failure into a hidden one.
   If the verification is wrong, that is a finding to report, not a thing to quietly fix.
4. **When the plan is wrong, stop — do not redesign.** Most mismatches are *not* the plan being
   wrong: a moved folder or a renamed helper is a detail you absorb and record. See **When the
   plan is wrong** for the one-question test that separates those from a design decision you must
   escalate.

## Constraints that hold in every project

- **Never hand-edit generated, vendored, or lock files.** Change the source and re-run the
  generator. If the plan tells you to edit a generated file, that is a defect in the plan.
- **Never commit secrets.** No credentials, tokens, or environment values in code, fixtures, logs,
  or the plan file. Reference the config key by name.
- **Follow the project's migration policy exactly** as the plan recorded it. Never substitute a
  more convenient command.
- **Do not commit, push, or open a pull request unless asked.** Leave the work in the tree; let
  the user decide when it lands.
- **A project's own rules outrank this skill.** If the repository documents a convention that
  contradicts anything here, follow the repository and note it.

## Scaling to the change

The plan recorded a **tier**. Read it and scale accordingly — do not re-classify. The planning
step made that judgment with the whole repository in view, and re-deciding it here just creates a
second opinion nobody reconciles.

| | Trivial | Standard | Structural |
| --- | --- | --- | --- |
| Phase 1 Preflight | Confirm every step has a `Verify:` | Full | Full |
| Phase 2 Ground truth | Confirm the files you will touch exist as described | Full | Full |
| Phase 3 Baseline | Risk-based (see the table there) | Risk-based | Full suite always |
| Phase 4 Step loop | **Unchanged** | **Unchanged** | **Unchanged** |
| Phase 5 Completion sweep | Diff review, no secrets, no debug artifacts, lint/format on touched files | Full | Full |
| Phase 6 Write-back | Outcome only if something deviated or the plan was wrong | Full | Full |
| Extra | — | — | Pause for user review once the step carrying the change named in the plan's tier reason is verified, before continuing |

If the plan records no tier — it predates this field, or was given inline — treat it as
**Standard**.

**The step loop never scales.** Every tier verifies each step before starting the next. That is
the guarantee this skill exists to provide; a "fast mode" that drops it is not a faster version of
this skill, it is the absence of it.

---

# The pipeline

## Phase 0 — Load the plan

Read the plan file end to end before touching any code. All of it — including the sections that
look like background. The Context, Architecture conformance, and Design sections are what keep
your code looking like it belongs in this repository; skipping to the Steps produces code that
satisfies each step and fits nowhere.

Note specifically: the **tier**, the **reference implementation**, the **hard constraints**, the
**deviations** already recorded, and the **verification plan** the QA step will use.

If there is no plan file — the change was tiered Trivial and planned inline — take the inline plan
as your input and run the same pipeline at Trivial scale.

**Gate:** you can state the goal, the tier, the step count, and the reference implementation
without re-reading.

## Phase 1 — Preflight

Confirm the plan is actually executable before you start:

- **Status** is `planned` or `in-progress`. If `blocked`, do not start — the blocking question
  must be answered first.
- **No unresolved `BLOCKING` questions** remain in Risks & open questions.
- **The verification gate ran.** If that section is empty, the plan skipped its own final check.
  Tell the user before proceeding; a plan that never faced its own checklist usually has gaps that
  surface halfway through implementation.
- **Every step has a verification.** A step without one cannot be completed under rule 2.
- **The plan does not contradict its own hard constraints section.**

If any fail, stop and report. Starting anyway means discovering the same problem later, with half
a diff in the tree.

**Gate:** all five confirmed (Trivial: the fourth only), or reported and explicitly waived.

## Phase 2 — Ground truth

The plan was written against the repository at a point in time. Confirm that point is still now.

- Check whether the area changed since the plan was written — compare the plan's date against
  recent history for the files it names.
- **Spot-check the plan's `path:line` anchors.** Open several and confirm they still point at what
  the plan says. Anchors drift silently, and a plan built on moved code will send you to the wrong
  place with total confidence.
- Confirm the files the plan says it will create do not already exist, and the ones it will edit
  do.
- Confirm the dependencies and versions the plan assumed are the ones installed.

Files you open here count as read for Phase 4's purposes until something writes to them.

If the ground has moved materially — the reference implementation was refactored, the module
moved, the schema changed — that is a plan-level problem, not something to absorb. Report it and
ask whether to re-plan.

**Gate:** anchors verified or drift reported.

## Phase 3 — Baseline

Know which failures are yours *before* you create any. Without a baseline, a pre-existing failure
becomes something you assume you caused, and you will spend a long time fixing code you never
touched.

How much to run depends on what the plan's steps actually touch:

| What the steps touch | Baseline to run |
| --- | --- |
| Documentation, comments, markdown only | None — go to Phase 4 |
| Static assets, UI copy, styling | Build |
| Application code | Build + type-check + lint + the tests covering the touched area |
| Schema or migrations, shared modules, build config, dependencies | Full suite |
| Anything, when the tier is Structural | Full suite |

Take the heavier row when a change spans two, and when you are unsure. Record what you ran and
what it said, including any pre-existing failures — and tell the user about those before
proceeding.

Work on a branch if the project uses them and you are not already on a suitable one.

**Gate:** baseline recorded at the right level, including pre-existing failures.

## Phase 4 — The step loop

The core of the skill. For each step in the plan, in order:

1. **Open the file you are about to edit if your copy may be stale.** Re-read when: you have not
   opened it in this session; anything has written to it since you last read it; a verification
   failed and you are about to change it again; or you are about to match on exact text you did
   not just see. If you read it in this session and nothing has touched it since, work from what
   you have. The failure this prevents is an edit applied to a version of the file that no longer
   exists — cheap to prevent, expensive to debug.
2. **Make the change the step describes** — the minimum that satisfies it. Not the step plus an
   improvement. Not the step plus a refactor of the function beside it.
3. **Match the reference implementation.** Naming, error handling, file layout, import style. When
   in doubt, open it and copy its shape.
4. **Run the step's `Verify:`.** The actual command, in the actual repository. Read the actual
   output.
5. **Assign a status** (see below), record it with a one-line note of what you observed, update
   Progress, and move on.
6. **On failure, see the retry rule below.**
7. **Leave the repository working.** If a step must temporarily break the build, the plan should
   have said so; if it did not, note it and keep the window short.

**Never start step N+1 while step N is unresolved** — meaning it has no status yet. A step marked
`blocked` or `deferred` is resolved: the debt is visible and carried forward.

### Step statuses

Verification is not binary, and pretending it is hides exactly the information QA needs. Assign
one, based on what actually ran — never on how confident you feel:

| Status | Means |
| --- | --- |
| `verified` | The step's own `Verify:` ran and passed |
| `verified-indirectly` | A broader check covers it — name which one and why it is sufficient |
| `blocked` | Cannot run here — name what is missing and what would close it |
| `deferred` | Cannot be verified until a later step lands — name which step |

There is deliberately no "high confidence" status. That is a feeling wearing a label, and it would
absorb every ambiguous case in the run.

### When a step fails

Retry, but under a discipline that distinguishes converging on a root cause from guessing:

- **State a hypothesis before each attempt** — one sentence: what you believe is wrong, and why
  this change addresses it.
- **The error signature is the counter.** Same error, same failing assertion, same stack = the
  same signature. A different signature means you learned something and the count resets.
- **Stop when the same signature repeats and you have no new hypothesis** — that is thrashing, and
  every further change makes the tree worse rather than better.
- **Backstop: stop after five attempts on one step regardless**, even if signatures keep changing.
  Endlessly shifting errors mean the changes are random, which is thrashing in a different
  costume.

When you stop, report the hypotheses you tried and what each ruled out. That list is worth more
than the diff.

**Gate per step:** a status assigned, and Progress updated.

## Phase 5 — Completion sweep

Before declaring anything done (Trivial tier: the starred items only):

- [ ] ★ **Nothing outside the plan changed.** Read your own diff in full. Anything no step called
      for gets removed, or gets flagged to the user with a reason.
- [ ] ★ **No debug artifacts** — stray logging, commented-out code, temporary files, scaffolding
      you added to get a step to pass.
- [ ] ★ **No secrets** anywhere in the diff.
- [ ] ★ **Lint, format, and type-check pass** on what you touched, using the project's commands.
- [ ] **The deletions happened.** The plan's Maintainability section named code this change makes
      dead. Additive-only implementations are how a codebase rots — if you left it, say why.
- [ ] **Full test suite** run and compared against the Phase 3 baseline — same failures, no new
      ones.
- [ ] **Generated artifacts regenerated** if the plan required it, using the project's command.
- [ ] **The acceptance criteria in the plan's Verification plan are met** — checked one by one, not
      inferred from the steps having passed.

**Gate:** every applicable box ticked or explicitly explained.

## Phase 6 — Write back to the plan

The plan file is the shared memory between planning, implementation, and QA. Keep it true.

**During Phase 4**, maintain the Progress section so an interrupted session can be resumed by
someone — including you, later — who has lost all context:

```markdown
## Progress
- [x] 1. <step name> — `verified`: <what you observed>
- [x] 2. <step name> — `verified-indirectly`: covered by <check>, sufficient because <why>
- [x] 3. <step name> — `deferred`: verifiable only after step 5
- [ ] 4. <step name> — in progress
- [ ] 5. <step name>
```

**At the end**, set `Status: done` (or leave `in-progress` if you stopped early) and fill in the
`## Outcome` section. Write it for someone about to plan a change in this same area who does not
know what you learned:

```markdown
## Outcome
**Deviations from the plan:** <what you did differently, and why>. Or "None".
**Plan defects:** <what was wrong, missing, or based on a false assumption>. Or "None".
**Steps not fully verified:** <which, their status, and what would close them>. Or "None".
**Left behind:** <known debt, skipped deletions, follow-up work>. Or "None".
**For the next plan in this area:** <what you wish the plan had told you>.
```

Be specific and be honest. A vague or flattering Outcome is worse than none, because the next plan
will trust it. "Plan assumed the repository exposed a transaction helper; it does not, so the
service manages its own connection" is useful. "Went well, minor issues" is noise.

At Trivial tier, write the Outcome only if something deviated or the plan was wrong. Nothing to
report is not worth a section.

**Gate:** Progress complete, Status correct, Outcome filled in where required.

## Phase 7 — Handoff

Report to the user:

- What was built, in two or three sentences
- Every deviation, and why
- Every step not `verified`, with its status and what would close it
- The list of things you noticed but did not touch
- What QA should focus on — where you are least confident

Then stop. Do not commit, push, or open a pull request unless asked.

---

# When the plan is wrong

It will happen. The test is one question: **would this change an answer in the plan's Design
section?**

**No → absorb it and record it.** Proceed, and note it in the Outcome.

> Step says to add the handler in `notifications/handlers/`, but the project moved handlers to
> `notifications/delivery/` last week. Same layer, same conventions, different folder. Put it in
> the right place and record the correction.

> Plan says to write a `formatCurrency` helper; one already exists in the shared utilities. Use
> the existing one — the plan's intent was the behaviour, not the file.

**Yes → stop and escalate.** Do not redesign in flight. Report what the plan assumed, what is
actually true, and what you would suggest instead — then wait.

> Plan's design depends on the ORM supporting nested transactions. It does not. Every option from
> here — restructure the service, use an outbox, accept partial writes — changes the design. That
> is a planning decision, not an implementation one.

**Borderline → treat it as material.** The cost of asking is a message; the cost of guessing is a
design nobody reviewed and nobody can find later, because it exists only in the diff.

# Stopping conditions

Stop and ask rather than continuing when:

- A step's failures hit the retry rule above
- The plan turns out to be materially wrong
- Ground truth has drifted enough that the plan's anchors no longer hold
- A step would require editing a generated file, bypassing the migration policy, or breaking a
  hard constraint
- The change needed to pass a step is larger than the step describes
- You are about to touch a file no step mentioned

# Done when

Every step carries a status, the completion sweep is clean at the tier's level, the plan file has
an accurate Progress, Status, and Outcome, and the user has the handoff report — including
everything you deliberately did not do.