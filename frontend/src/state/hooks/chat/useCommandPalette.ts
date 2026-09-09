import { useCallback, useEffect, useMemo, useRef, useState } from "preact/hooks";
import type { ChatProvider } from "../../../models/chat";
import type { RegisteredSkill } from "../../../models/skill";
import { commandPaletteState } from "./commandPaletteState";
import { useAvailableSkills } from "./useAvailableSkills";

export function useCommandPalette({
  provider,
  projectId,
  query,
  onSelect,
  onDismiss,
}: {
  provider: ChatProvider;
  projectId?: string;
  query: string;
  onSelect: (skill: RegisteredSkill) => void;
  onDismiss: () => void;
}) {
  const { skills, loading, error } = useAvailableSkills(provider, projectId);
  const [highlighted, setHighlighted] = useState(0);
  const items = useMemo(
    () => commandPaletteState.filter(skills, query),
    [skills, query],
  );
  const itemsRef = useRef<RegisteredSkill[]>([]);
  itemsRef.current = items;

  useEffect(() => {
    setHighlighted(0);
  }, [items.length]);

  const choose = useCallback((skill: RegisteredSkill) => {
    onSelect(skill);
    onDismiss();
  }, [onSelect, onDismiss]);

  const highlight = useCallback((index: number) => {
    setHighlighted(index);
  }, []);

  const handleKeyDown = useCallback((event: KeyboardEvent): boolean => {
    const currentItems = itemsRef.current;
    switch (commandPaletteState.actionForKey(event.key, currentItems.length)) {
      case "dismiss":
        event.preventDefault();
        onDismiss();
        return true;
      case "next":
        event.preventDefault();
        setHighlighted((current) =>
          commandPaletteState.moveHighlight(current, 1, currentItems.length)
        );
        return true;
      case "previous":
        event.preventDefault();
        setHighlighted((current) =>
          commandPaletteState.moveHighlight(current, -1, currentItems.length)
        );
        return true;
      case "choose": {
        const selected = commandPaletteState.selectedItem(currentItems, highlighted);
        if (!selected) return false;
        event.preventDefault();
        choose(selected);
        return true;
      }
      case "ignore":
        return false;
    }
  }, [choose, highlighted, onDismiss]);

  return {
    items,
    registeredCount: skills.length,
    loading,
    error,
    highlighted,
    onHighlight: highlight,
    onChoose: choose,
    handleKeyDown,
  };
}
