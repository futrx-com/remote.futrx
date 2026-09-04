import type { RegisteredSkill } from "../../../models/skill";

const SLASH_PATTERN = /^\/(\S*)$/;

export type SlashCommandKeyAction = "next" | "previous" | "choose" | "dismiss" | "ignore";

class SlashCommandMenuPolicy {
  resolve(text: string, skills: RegisteredSkill[]) {
    const match = SLASH_PATTERN.exec(text);
    if (!match) return null;

    const query = match[1];
    const term = query.trim().toLowerCase();
    if (!term) return { query, items: skills };

    return {
      query,
      items: skills.filter((skill) =>
        `${skill.name} ${skill.command || ""} ${skill.description || ""} ${skill.source || ""}`
          .toLowerCase()
          .includes(term)
      ),
    };
  }

  clampHighlight(highlight: number, itemCount: number): number {
    return itemCount ? Math.min(highlight, itemCount - 1) : 0;
  }

  moveHighlight(highlight: number, step: -1 | 1, itemCount: number): number {
    return itemCount ? (highlight + step + itemCount) % itemCount : 0;
  }

  actionForKey(
    keyPress: { key: string; shiftKey: boolean; ctrlKey: boolean; metaKey: boolean },
    itemCount: number,
  ): SlashCommandKeyAction {
    switch (keyPress.key) {
      case "ArrowDown":
        return "next";
      case "ArrowUp":
        return "previous";
      case "Tab":
        return itemCount ? "choose" : "ignore";
      case "Enter":
        if (keyPress.shiftKey || keyPress.ctrlKey || keyPress.metaKey) return "ignore";
        return itemCount ? "choose" : "ignore";
      case "Escape":
        return "dismiss";
      default:
        return "ignore";
    }
  }
}

export const slashCommandMenuPolicy = new SlashCommandMenuPolicy();
