import type { RegisteredSkill } from "../../../models/skill";

type CommandPaletteKeyAction = "dismiss" | "next" | "previous" | "choose" | "ignore";

class CommandPaletteState {
  query(text: string): string | null {
    if (text.length < 1 || !text.startsWith("/")) return null;
    return text.slice(1);
  }

  filter(skills: RegisteredSkill[], query: string | null): RegisteredSkill[] {
    const term = (query ?? "").trim().toLowerCase();
    if (!term) return skills;

    const commandMatches = skills.filter((skill) =>
      this.commandTerm(skill).startsWith(term)
    );
    if (commandMatches.length > 0) return commandMatches;

    return skills.filter((skill) =>
      `${this.commandTerm(skill)} ${skill.name} ${skill.description || ""}`
        .toLowerCase()
        .includes(term)
    );
  }

  actionForKey(key: string, itemCount: number): CommandPaletteKeyAction {
    if (key === "Escape") return "dismiss";
    if (itemCount === 0) return "ignore";

    switch (key) {
      case "ArrowDown":
        return "next";
      case "ArrowUp":
        return "previous";
      case "Enter":
      case "Tab":
        return "choose";
      default:
        return "ignore";
    }
  }

  moveHighlight(highlight: number, step: -1 | 1, itemCount: number): number {
    return itemCount ? (highlight + step + itemCount) % itemCount : 0;
  }

  selectedItem(items: RegisteredSkill[], highlight: number): RegisteredSkill | undefined {
    return items[Math.min(highlight, items.length - 1)];
  }

  private commandTerm(skill: RegisteredSkill): string {
    return (skill.command || skill.name).replace(/^\//, "").toLowerCase();
  }
}

export const commandPaletteState = new CommandPaletteState();
