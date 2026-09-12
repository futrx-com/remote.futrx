---
name: code-review-guard
description: "Rigorous review checklist for production code in any language - Clean Code, SOLID, DRY, KISS and the security basics (input validation, output escaping, prepared statements, secrets handling, authorization). Use after writing, editing, refactoring or fixing code and before presenting, committing or merging it, and when asked to 'review this', 'audit this code' or 'is this safe to merge'."
---

# Code review guard

Review the change that is actually in front of you. Read the diff and the
files it touches before judging anything. Never invent findings, and never
approve code you have not read.

## How to run a review

1. Establish intent: what is this change supposed to do?
2. Read the full diff, then open each touched file for surrounding context.
3. Walk the checklists below and collect concrete findings.
4. Report findings ordered **blocker → major → minor → nit**, each with the
   file, the line, why it matters, and the smallest fix that resolves it.
5. State clearly whether the change is safe to merge.

If a checklist item does not apply to this change, skip it silently. A short
review with three real findings beats a long one padded with generalities.

## Correctness first

- Does the code do what the change description claims?
- Boundary conditions: empty input, single element, maximum size, zero,
  negative, unicode, timezone edges.
- Every error path is handled or deliberately propagated. No silently
  swallowed errors, no `catch {}` that hides a failure.
- Resources (files, sockets, locks, transactions) are released on every path,
  including early returns and panics.
- Concurrency: shared mutable state is guarded; no data race, no lock ordering
  that can deadlock, no unbounded goroutine or thread spawning.
- Behaviour that changed is covered by a test that would fail without the fix.

## Clean Code, SOLID, DRY, KISS

- **Names** say what the thing is or does. No `data`, `tmp`, `handle2`,
  no abbreviations the reader must decode.
- **Functions** do one thing at one level of abstraction. A function that
  needs a comment to explain its second half wants to be two functions.
- **Single responsibility**: a type or module has exactly one reason to
  change. Watch for classes that both compute and persist.
- **Open/closed and dependency inversion**: depend on the narrow interface you
  need, not on a concrete implementation or a god object.
- **Interface segregation**: callers should not be forced to know about
  methods they never call.
- **DRY**: the same decision must not be encoded in two places. Duplicated
  *knowledge* is the problem; incidental similarity is not — do not merge two
  functions that just happen to look alike today.
- **KISS / YAGNI**: no abstraction layer, plugin registry, configuration
  switch or generic parameter added for a requirement nobody has asked for.
- **Comments** explain *why*, never *what*. Delete commented-out code.
- **Dead code**: unused exports, parameters, branches and flags come out.

## LLM-specific failure modes

Machine-written code fails in recognizable ways. Check for:

- Invented APIs, flags, or config keys that do not exist in this codebase or
  its dependencies — verify every symbol against the source.
- Plausible-looking constants (timeouts, limits, magic numbers) with no basis.
- Duplicated helpers that already exist elsewhere in the repository.
- Tests that assert the implementation back to itself, or that pass whatever
  the code does (`assert result == result`, mocks asserting on mocks).
- Broad `try`/`except` or `if err != nil { return nil }` that converts a real
  failure into a wrong-but-quiet success.
- Scope creep: unrelated reformatting, renames, or "while I was here" edits
  mixed into a focused change.

## Security basics

- **Input validation**: validate on the server, at the trust boundary, against
  an allowlist. Never trust client-side checks, hidden fields, or ordering.
- **Output escaping**: escape at the point of output and for the correct
  context — HTML body, HTML attribute, URL, JavaScript, shell, SQL. Never
  build HTML by string concatenation of user data.
- **SQL**: parameterized statements or prepared queries only. No string
  interpolation into a query, including for `ORDER BY` and table names —
  map those through an allowlist instead.
- **Command execution**: pass argument arrays, never a shell string built
  from user input.
- **Path handling**: resolve and confirm the result stays inside the intended
  root before opening anything derived from user input.
- **Deserialization**: never deserialize untrusted data into arbitrary types.
- **AuthN/AuthZ**: every privileged action re-checks the caller's identity
  *and* their permission for that specific object. Do not rely on the UI
  hiding a button.
- **Secrets**: no credentials, tokens, or keys in source, logs, error
  messages, or client-visible responses. Read them from the environment or a
  secret store. Rotate anything that leaked.
- **Crypto**: use vetted library primitives. No hand-rolled crypto, no MD5 or
  SHA-1 for passwords, no ECB, no static IVs. Use a password hash designed for
  it (argon2id, scrypt, bcrypt).
- **Randomness**: security decisions use a cryptographically secure source.
- **Errors and logging**: log enough to debug, never enough to leak. No PII,
  no tokens, no full request bodies.
- **Dependencies**: new dependencies are justified, pinned, and maintained.

## Report format

```
BLOCKER  path/to/file.go:42 — Unparameterized query interpolates `userID`.
         Use a prepared statement: db.Query("... WHERE id = ?", userID)
MAJOR    path/to/service.ts:88 — Error from saveOrder() is swallowed; the
         caller reports success on a failed write.
MINOR    path/to/util.py:12 — `process()` does three unrelated things; split
         out the validation step.
NIT      path/to/file.go:7 — Exported symbol lacks a doc comment.
```

End with one line: **Safe to merge** or **Changes requested**, and why.
