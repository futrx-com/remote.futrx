// Which skills a typed query finds, for the `/` command palette and the skill
// picker alike.
//
// Both surfaces used to test `text.includes(query)`, which treated the query
// as one literal string: "ui ux" could never find `ui-ux-pro-max`, because the
// skill spells the gap with a hyphen. Matching runs on tokens instead, so
// separators, word order within a field, and small typos stop mattering.

import type { RegisteredSkill } from "../../models/skill.ts";
import { textFoldService } from "../platform/textFoldService.ts";
import { textMatchService } from "../platform/textMatchService.ts";

/**
 * What a hit in each field is worth, as a multiplier on how well it matched.
 * The command is what the user is typing, so it weighs most, then the display
 * name, then the prose. This is a weighting, not a strict order: an exact name
 * hit still beats a typo-level command hit, the same trade-off the sidebar's
 * SEARCH_FIELD_WEIGHTS makes.
 */
const FIELD_WEIGHTS = { command: 3, name: 2, description: 1, source: 0.5 } as const;

type SkillField = keyof typeof FIELD_WEIGHTS;

class SkillSearchService {
  /**
   * Skills whose command starts with the query, in their original order, or
   * -- when none does -- every skill any of whose fields matches, best first.
   * Prefix hits win outright so a short query like "b" lists `/build` alone
   * rather than every skill with a "b" somewhere in its description.
   */
  filter(skills: RegisteredSkill[], query: string | null): RegisteredSkill[] {
    const raw = (query ?? "").trim();
    if (!raw) return skills;
    const tokens = textMatchService.tokenize(raw);

    const prefixHits = skills.filter((skill) => this.#commandStartsWith(skill, raw, tokens));
    if (prefixHits.length > 0) return prefixHits;

    return skills
      .map((skill) => ({ skill, score: this.#score(skill, tokens) }))
      .filter((hit) => hit.score > 0)
      .sort((left, right) => right.score - left.score)
      .map((hit) => hit.skill);
  }

  /** The command without its leading slash, falling back to the name. */
  #commandTerm(skill: RegisteredSkill): string {
    return (skill.command || skill.name).replace(/^\//, "");
  }

  /**
   * True when the query reads as the start of the command. Token-wise, every
   * query word but the last must equal the command's word in that position and
   * the last must begin it, so "ui ux" and "ui-u" both start `ui-ux-pro-max`.
   */
  #commandStartsWith(skill: RegisteredSkill, raw: string, tokens: string[]): boolean {
    const command = this.#commandTerm(skill);
    if (textFoldService.fold(command).startsWith(textFoldService.fold(raw))) return true;
    if (tokens.length === 0) return false;

    const words = textMatchService.tokenize(command);
    if (tokens.length > words.length) return false;
    const last = tokens.length - 1;
    for (let i = 0; i < last; i += 1) {
      if (words[i] !== tokens[i]) return false;
    }
    return words[last].startsWith(tokens[last]);
  }

  /** The best weighted field score, or 0 when no field matches every token. */
  #score(skill: RegisteredSkill, tokens: string[]): number {
    let best = 0;
    for (const field of Object.keys(FIELD_WEIGHTS) as SkillField[]) {
      const hit = textMatchService.matchField(textFoldService.fold(this.#text(skill, field)), tokens);
      if (hit) best = Math.max(best, hit.score * FIELD_WEIGHTS[field]);
    }
    return best;
  }

  #text(skill: RegisteredSkill, field: SkillField): string {
    switch (field) {
      case "command":
        return this.#commandTerm(skill);
      case "name":
        return skill.name;
      case "description":
        return skill.description || "";
      case "source":
        return skill.source || "";
    }
  }
}

export const skillSearchService = new SkillSearchService();
