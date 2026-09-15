import { forwardRef } from "preact/compat";
import { useImperativeHandle } from "preact/hooks";
import type { ChatProvider } from "../../../models/chat";
import type { RegisteredSkill } from "../../../models/skill";
import { useCommandPalette } from "../../../state/hooks/chat/useCommandPalette";
import { CommandPaletteView } from "./CommandPaletteView";

export interface CommandPaletteHandle {
  /** Returns whether the open palette consumed the composer's keyboard event. */
  handleKeyDown(event: KeyboardEvent): boolean;
}

export const CommandPalette = forwardRef<CommandPaletteHandle, {
  provider: ChatProvider;
  projectId?: string;
  query: string;
  onSelect: (skill: RegisteredSkill) => void;
  onDismiss: () => void;
}>(
  function CommandPalette({ provider, projectId, query, onSelect, onDismiss }, ref) {
    const palette = useCommandPalette({
      provider,
      projectId,
      query,
      onSelect,
      onDismiss,
    });

    useImperativeHandle(ref, () => ({
      handleKeyDown: palette.handleKeyDown,
    }), [palette.handleKeyDown]);

    return (
      <CommandPaletteView
        query={query}
        items={palette.items}
        registeredCount={palette.registeredCount}
        highlighted={palette.highlighted}
        loading={palette.loading}
        error={palette.error}
        onChoose={palette.onChoose}
        onHighlight={palette.onHighlight}
      />
    );
  }
);
