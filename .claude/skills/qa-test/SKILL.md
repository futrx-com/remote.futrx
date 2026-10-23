---
name: qa-test
description: Independently verify an implementation against the plan it came from — running the acceptance criteria, attacking the change adversarially, checking for regressions, and reporting defects. Use when the user says "QA", "test it", "verify the implementation", "check this works", "review the change", or after an implementation finishes and the work needs checking before it ships. Reads the plan's verification criteria and the implementer's own report of what it could not verify, re-runs everything itself rather than trusting the report, tries to break the change rather than confirm it, and writes a verdict back into the plan file. Not for writing the feature or fixing what it finds — defects go back to `implement`.
---

# QA-testing an implementation

Implementation verified that each step did what the step said. That is not the same as the feature
working. Steps pass individually while the whole is broken; the happy path works while every
error path is unhandled; the new code is correct while the code that called the old code is not.
Your job is the part step-verification structurally cannot reach.

You are also the only independent reader in this pipeline. The implementer wanted its work to
pass — not dishonestly, but because that is what building something does to your attention. You
want to find what is wrong. Hold that asymmetry deliberately: it is the entire value you add, and
it evaporates the moment you start taking the implementer's word for things.

**Budget: find the problem now.** A defect caught here costs a message. The same defect caught in
production costs an incident and a rollback.

## Four rules that outrank everything else

1. **Trust nothing; re-run it yourself.** The Progress and Outcome sections are a *map of where to
   look*, not evidence. A step marked `verified` is a claim. Re-run it.
2. **Try to break it.** A QA pass that walks the happy path and reports success has tested that
   the code compiles. Attack the boundaries, the error paths, the concurrent cases, the data that
   already exists.
3. **Report defects; do not fix them.** The moment you fix something, you are the implementer, and
   nobody independent is checking that fix. This holds even when the fix is one line and obvious —
   *especially* then, because that is when it is most tempting and least examined.
4. **Evidence or it did not happen.** Every finding carries reproduction steps and the actual
   output you saw. Every pass carries what you ran. "Looks correct" is not a result.

## Independence is structural, not an attitude

Rules 1 and 2 ask you to be adversarial. Willingness is not enough. If you also implemented this
change, its reasoning is already in your context, and you will re-derive the same blind spots —
because they are yours. You cannot decide your way out of knowing why the code is shaped the way
it is, and an agent reviewing its own work reliably finds the defects it was already looking for
and misses the ones it never considered.

So establish independence structurally, in this order of preference:

1. **Fresh context.** Run QA in a session that did not implement the change, given the plan file
   and the diff and nothing else.
2. **Delegated.** If you can spawn a subagent, give it the plan path and the diff — no
   implementation history, no explanation of why the design is what it is.
3. **Disclosed.** If neither is available, proceed — but record `Independence: same session as
   implementation` in the verdict, and treat every "that is fine, I know why it does that"
   reaction as a signal to test the thing rather than skip it. That reaction is precisely the
   knowledge an independent tester would not have.

A disclosed loss of independence is a known limitation the reader can weigh. An undisclosed one
produces a clean report that means nothing, and nobody downstream can tell the difference.

## Constraints that hold in every project

- **Never edit source, tests, or fixtures to make something pass.** If a test is wrong, that is a
  finding.
- **Never test against production data or credentials.** If the only way to exercise a path is
  against production, that is a finding about the change, not a licence.
- **Never commit or push.** You are reading and running, not writing.
- **Destructive checks stay reversible.** Before testing a migration, know how to get back. If you
  cannot get back, do not run it — report that the rollback path is untestable.
- **A project's own rules outrank this skill.**

## Scaling to the change

Read the **tier** the plan recorded and scale. Do not re-classify.

| | Trivial | Standard | Structural |
| --- | --- | --- | --- |
| Phase 2 Diff review | Full | Full | Full |
| Phase 3 Acceptance criteria | Full | Full | Full |
| Phase 4 Adversarial | Error paths of the changed surface | Full checklist on the changed surface | Full checklist + seams + rollback |
| Phase 5 Regression | Tests covering the touched area | Full suite vs baseline | Full suite + every caller of a changed interface |
| Phase 6 Hygiene | Full | Full | Full |

Phases 2, 3, and 6 never scale down: they are cheap, and they catch the failures that embarrass
everyone. If the plan records no tier, treat it as **Standard**.

---

# The pipeline

## Phase 0 — Load the contract

Read the plan file. You need:

- **Verification plan** — the acceptance criteria. This is your primary contract.
- **Progress** — every step's status. `blocked` and `deferred` steps are *your inheritance*: they
  were never verified by anyone, and they are the highest-yield place to start.
- **Outcome** — deviations, plan defects, what was left behind, and where the implementer said it
  was least confident. Treat that last one as a map of the weak points, because it usually is.
- **Risks & open questions** — each risk names something that could go wrong. Test whether it did.
- **Hard constraints** — a change that violates one is a blocker regardless of whether it works.
- **Reference implementation** and **tier**.

If there is no plan file, ask for the acceptance criteria before testing. Testing without a
contract produces opinions about the code, not a verdict on the change.

**Gate:** you can state the acceptance criteria, the unverified steps, and the declared weak points.

## Phase 1 — Independent baseline

You need to know which failures pre-date this change. Running the suite now cannot tell you that —
the change is already in the tree, so every failure you see is ambiguous. The baseline has to come
from the code as it was.

Method:

1. **Identify the base revision** — the commit the change started from. The plan's date, the
   branch point, or the last commit before the implementation's first commit will give it to you.
2. **Run the suite there, without disturbing the working tree.** A separate checkout of the base
   revision is the clean way — most version-control systems can give you one in a second directory
   rather than making you stash and switch. Install dependencies at the versions that revision
   pins; a stale dependency tree produces failures that belong to neither the base nor the change.
3. **Record what fails there.** Those failures are not yours and not the implementer's.
4. **Return to the change** and run the same suite the same way. Only differences between the two
   runs belong to this change.

Compare your result against the baseline the implementer recorded. If they disagree, that
discrepancy is a finding in itself — one of you has an environment problem, and you need to know
which before anything else you report can be trusted.

**If you cannot obtain a base revision** — no version control, an unrevertable tree, a build that
cannot be reproduced at that commit — say so. Treat the implementer's recorded baseline as
unverified input, and mark any failure you cannot confidently attribute as `blocked` rather than
guessing whose it is. A misattributed failure wastes more time than an unattributed one.

**Gate:** baseline established at the base revision, or explicitly recorded as unobtainable with
the consequence stated.

## Phase 2 — Diff review

Read the entire diff before running anything against the feature. This is the highest-value
twenty minutes in the pipeline, and it is the one thing no test suite does for you.

*Why before the acceptance criteria:* the criteria were written in advance and are objective, so
reading the diff first biases them very little. The diff, on the other hand, is the only place
scope creep, missing deletions, leaked secrets, and silent convention drift are visible at all —
no criterion covers them, because nobody knew to write one. Running criteria first also tends to
produce a "green, so we're done" reflex that makes the diff read shallower. If your team prefers
the opposite order to keep the reviewer maximally naive, that is defensible — just do not drop the
diff review.

Check:

- **Scope:** does anything in the diff correspond to no step in the plan? Unexplained changes are
  findings even when they are improvements.
- **Deletions:** the plan's Maintainability section named code this change makes dead. Is it gone?
- **Conformance:** does the new code look like the reference implementation — naming, error
  handling, layering, imports? Divergence here is a real defect; it is the mechanism by which
  codebases become inconsistent one reasonable change at a time.
- **Leftovers:** debug logging, commented-out code, `TODO`s added by this change, temporary files,
  scaffolding.
- **Secrets:** credentials, tokens, real endpoints, production identifiers — in code, fixtures,
  test data, and log lines.
- **Generated and vendored files:** edited by hand, or properly regenerated?
- **The things nobody looks at:** error messages, log levels, config defaults, migration
  down-paths.

**Gate:** every diff hunk is either explained by a plan step or recorded as a finding.

## Phase 3 — Acceptance criteria

Take the plan's Verification plan and work through it one criterion at a time. For each: run it,
record the command or action, and record the actual output.

Rules:

- **One criterion, one result.** Do not batch several criteria into a single judgment.
- **A criterion you cannot run is `blocked`, not passed.** Name what is missing.
- **A criterion that is vague is a finding.** "The feature works correctly" cannot be tested; that
  is a defect in the plan, and it goes in the report.
- **Then verify the steps the implementer could not.** Everything marked `blocked` or `deferred`
  in Progress, plus anything marked `verified-indirectly` where the indirect check looks thinner
  than the claim it supports.

**Gate:** every criterion has a result and evidence.

## Phase 4 — Adversarial pass

Now stop confirming and start attacking. Work the checklist against the surface this change
touches — skip a row only when it genuinely cannot apply, and say which and why.

- **Empty and missing** — null, empty string, empty collection, absent optional field, absent
  required field.
- **Boundaries** — zero, one, maximum, one past maximum, negative, very long strings, unicode.
- **Malformed** — wrong type, wrong shape, extra fields, truncated payload.
- **Authorization** — a different role, no role, another tenant's identifier. Can the wrong user
  reach this?
- **Repetition and concurrency** — call it twice, submit twice, run two at once. Is it idempotent?
  Does it double-write?
- **Dependency failure** — the external service times out, returns 500, returns a partial or
  unexpected response. Does the failure surface, or get swallowed?
- **Partial failure** — interrupt it midway. Is the persisted state consistent, or half-written?
- **Pre-existing data** — records created before this change. Does the new code handle the old
  shape?
- **Volume** — enough data to expose an N+1 query or an unbounded fetch.
- **Rollback** — if there is a migration, run the down path on a disposable copy. An untested
  rollback is not a rollback.

The bar is not "did I try these" but "did I try to make it fail". If everything passed on the
first attempt, you were probably testing politely.

**Gate:** each row run or explicitly skipped with a reason; every failure recorded with
reproduction steps.

## Phase 5 — Regression and seams

Defects cluster at the joins, because that is where two people's assumptions meet.

- **Full suite** against the Phase 1 baseline. Any new failure is a blocker until explained.
- **Callers of anything whose signature, return shape, error behaviour, or nullability changed.**
  Find them by searching, not by remembering.
- **The reference implementation's sibling feature** — if the change touched shared code, the
  feature it was modelled on is the most likely casualty.
- **The public surface** — API responses, event payloads, exported types. Did the shape change in
  a way no criterion covers and no consumer expects?
- **Configuration and startup** — does the application still boot from a clean checkout with only
  the documented setup?

**Gate:** suite compared, callers checked, no unexplained new failure.

## Phase 6 — Hygiene and safety

Quick, mechanical, and the source of most avoidable production incidents:

- [ ] Lint, format, and type-check pass using the project's own commands
- [ ] No secrets in code, fixtures, logs, or test output
- [ ] No new dependency that the plan did not justify
- [ ] Migrations follow the project's documented policy — and are reversible, or documented as not
- [ ] Errors surface with enough context to debug, and without leaking internals to a caller
- [ ] Logging is at an appropriate level and does not print user data or credentials
- [ ] Nothing in the change violates a hard constraint from the plan

**Gate:** every box checked.

## Phase 7 — Verdict and write-back

Write a `## QA` section into the plan file. QA runs more than once — a `fail` sends the change back
to `implement`, which sends it back to you — so **append a round; never edit or replace an earlier
one.** The history is the point: a defect found in round one, fixed, and reappearing in round
three is the most valuable thing this record can tell anyone, and overwriting destroys exactly
that signal.

Before writing a finding, check it against earlier rounds. If the reproduction matches something
already reported, mark it `regression — first found in Round N`. A reintroduced defect is more
serious than a new one of the same severity: it means the fix did not hold, and the next fix
needs to explain why this one will.

```markdown
## QA
### Round 1 — <date>
**Verdict:** pass | pass with findings | fail | blocked
**Independence:** fresh context | delegated | same session as implementation — findings may be biased
**Tested at tier:** <tier>
**Baseline:** verified at <base revision> | unobtainable — <why, and what that leaves unattributed>
**Ran:** <suite, criteria count, adversarial rows covered, rollback tested y/n>

#### Findings
| # | Severity | What | Reproduce | Evidence |
| --- | --- | --- | --- | --- |
| 1 | blocker | <what is wrong> | <steps> | <actual output> |

#### Not tested
- <what, why it could not be tested, and what would close it>

#### For the next plan in this area
- <what QA wishes the plan had specified>

### Round 2 — <date>
<same shape; earlier rounds left untouched>
```

### Severity

Assign honestly. Inflating severity trains people to ignore your reports; deflating it gets things
shipped that should not be.

| Severity | Means |
| --- | --- |
| `blocker` | An acceptance criterion fails, data can be lost or corrupted, a security or authorization hole exists, or a hard constraint is violated |
| `major` | Works on the happy path but is wrong in a case real users will hit |
| `minor` | Real but narrow — cosmetic, or an edge case that is unlikely and non-damaging |
| `observation` | Not a defect in this change. Something the next plan should know |

### Verdict

- **pass** — every criterion passed, no blocker or major, no unexplained new suite failure
- **pass with findings** — as above, but minors or observations exist and are recorded
- **fail** — any blocker or major
- **blocked** — could not test enough of the change to have an opinion. Say exactly what is
  missing. A blocked verdict is a legitimate result; a guessed pass is not

Then report to the user: the verdict, the findings in severity order, what you could not test, and
the one thing you would look at hardest before this ships.

**Gate:** a new round appended under `## QA`, verdict stated, earlier rounds untouched, every
finding carrying evidence.

---

# When you find a defect

Report it; do not fix it. Rule 3 exists because a QA pass that also patches things leaves nobody
independent to check the patch, and because the fix belongs where the design context is.

Route it by severity:

- **blocker or major** → back to `implement`, with the plan reference and your reproduction steps.
  If the defect means the plan's design is wrong rather than its execution, say so — that goes
  back to `plan`, not `implement`.
- **minor** → recorded in Findings; the user decides whether it ships.
- **observation** → recorded, and surfaced for the next planning cycle.

The one thing you may write is a *failing test that reproduces the defect*, if the project has a
test suite and the user wants one. Add it failing; do not make it pass.

# When you cannot test

Some things cannot be exercised where you are — a third-party sandbox, a device, a production-only
integration, a load profile you cannot generate.

Say so precisely. Name what is untestable, why, what risk remains unverified, and what would close
it — a staging environment, a credential, a manual check by someone who has one. Then reflect it
in the verdict: enough untested surface makes the honest verdict `blocked`, not `pass`.

An untested path reported as untested is a known risk. An untested path reported as passing is a
false negative that everyone downstream will build on.

# Done when

Every acceptance criterion has a result with evidence, the adversarial checklist is covered or
explicitly skipped with reasons, the suite has been compared against the base revision, a new
round is appended under `## QA` with a verdict and an independence status, and the user knows what
you could not test.