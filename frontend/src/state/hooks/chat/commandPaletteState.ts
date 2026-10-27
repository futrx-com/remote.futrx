import type { RegisteredSkill } from "../../../models/skill";
import { skillSearchService } from "../../../services/chat/skillSearchService.ts";

type CommandPaletteKeyAction = "dismiss" | "next" | "previous" | "choose" | "ignore";

class CommandPaletteState {
  query(text: string): string | null {
    if (text.length < 1 || !text.startsWith("/")) return null;
    return text.slice(1);
  }

  filter(skills: RegisteredSkill[], query: string | null): RegisteredSkill[] {
    return skillSearchService.filter(skills, query);
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
}

export const commandPaletteState = new CommandPaletteState();
