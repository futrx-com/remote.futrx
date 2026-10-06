# `.claude/` — shared project brain (committed to git)

This folder is **checked into the repository on purpose** so that every developer (and every Claude
Code / AI assistant session) works from the same context and the same conventions. When you clone or
pull, you get the accumulated project knowledge; when you push, you share what you learned.

## What's here

| Path | Purpose |
|---|---|
| `memory/` | Durable facts about *this* project — decisions, gotchas, and "why" that the code alone doesn't explain. Start at `memory/MEMORY.md` (the index). |
| `skills/` | Repeatable recipes that keep everyone building the same way (e.g. how to add an LMS feature or a new bounded context). Each skill is a folder with a `SKILL.md`. |

## How to use it

- **Reading:** skim `memory/MEMORY.md` first; it links to one file per fact. Open the skill that
  matches your task before you start coding.
- **Writing:** when you make a non-obvious decision or hit a gotcha, add a short `memory/*.md` file
  and a line in `MEMORY.md`. When you establish a repeatable procedure, add a `skills/<name>/SKILL.md`.
- **Keep it small:** one fact per memory file; link related facts with `[[file-name-without-ext]]`.
  Don't duplicate what the code or README already says.

> Convention: keep this folder provider-neutral and human-readable. It is documentation first; the
> fact that assistants also read it is a bonus.
