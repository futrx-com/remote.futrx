import type { RegisteredSkill } from "../../../models/skill";

/**
 * Returns the filter query when the composer text is a command invocation
 * (starts with "/"), otherwise `null` to signal the palette should stay hidden.
 *
 * The palette only takes over when the "/" term is at the very start of the
 * draft, so typing "/" anywhere mid-sentence keeps behaving like a plain
 * character. Only a leader "/" advertises commands.
 */
export function commandQuery(text: string): string | null {
  if (text.length < 1 || !text.startsWith("/")) return null;
  return text.slice(1);
}

/**
 * Filters the registered skills by the command palette query, matching the
 * command name first and falling back to a broad name/description search.
 * Always returns at least the full list when there is no query so users can
 * browse every command after pressing "/".
 */
export function filterCommands(
  skills: RegisteredSkill[],
  query: string | null,
): RegisteredSkill[] {
  const term = (query ?? "").trim().toLowerCase();
  if (!term) return skills;
  const exact = skills.filter((skill) =>
    commandTerm(skill).startsWith(term)
  );
  if (exact.length > 0) return exact;
  return skills.filter((skill) =>
    `${commandTerm(skill)} ${skill.name} ${skill.description || ""}`.toLowerCase().includes(term)
  );
}

function commandTerm(skill: RegisteredSkill): string {
  return (skill.command || skill.name).replace(/^\//, "").toLowerCase();
}

