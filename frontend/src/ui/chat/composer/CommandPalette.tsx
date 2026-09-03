import { forwardRef } from "preact/compat";
import { useEffect, useImperativeHandle, useLayoutEffect, useMemo, useRef, useState } from "preact/hooks";
import type { ChatProvider } from "../../../models/chat";
import type { RegisteredSkill } from "../../../models/skill";
import { useAvailableSkills } from "../../../state/hooks/chat/useAvailableSkills";
import { Code } from "../../primitives/icons";
import { filterCommands } from "./commandQuery";

const MAX_VISIBLE = 8;

export interface CommandPaletteHandle {
  /**
   * Handles a keyboard event while the palette is open. Returns `true` when
   * the event was consumed by the palette (navigation/selection/dismiss).
   */
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
    const { skills, loading, error } = useAvailableSkills(provider, projectId);
    const [highlighted, setHighlighted] = useState(0);
    const [position, setPosition] = useState<{ left: number; top: number; width: number } | null>(null);
    const resultsRef = useRef<RegisteredSkill[]>([]);
    const listRef = useRef<HTMLDivElement>(null);

    const results = useMemo(
      () => filterCommands(skills, query),
      [skills, query],
    );
    resultsRef.current = results;

    // Keep the highlighted row inside bounds as the query narrows the results.
    useEffect(() => {
      setHighlighted(0);
    }, [results.length]);

    useEffect(() => {
      if (results.length === 0) return;
      listRef.current
        ?.querySelector<HTMLElement>(`[data-index="${highlighted}"]`)
        ?.scrollIntoView({ block: "nearest" });
    }, [highlighted, results.length]);

    useImperativeHandle(ref, () => ({
      handleKeyDown(event: KeyboardEvent): boolean {
        const items = resultsRef.current;
        if (event.key === "Escape") {
          event.preventDefault();
          onDismiss();
          return true;
        }
        if (items.length === 0) return false;
        if (event.key === "ArrowDown" || event.key === "ArrowUp") {
          event.preventDefault();
          if (event.key === "ArrowDown") {
            setHighlighted((current) => (current + 1) % items.length);
          } else {
            setHighlighted((current) => (current - 1 + items.length) % items.length);
          }
          return true;
        }
        if (event.key === "Enter" || event.key === "Tab") {
          const chosen = items[Math.min(highlighted, items.length - 1)];
          if (!chosen) return false;
          event.preventDefault();
          onSelect(chosen);
          onDismiss();
          return true;
        }
        return false;
      },
    }), [highlighted, onSelect, onDismiss]);

    // Pin to the viewport and keep it above the composer card, mirroring SkillPicker
    // (the thread column has overflow-hidden, so absolute positioning would clip).
    useLayoutEffect(() => {
      function place() {
        const card = document.querySelector<HTMLElement>(".codex-composer-card");
        if (!card) return;
        const bounds = card.getBoundingClientRect();
        setPosition({
          left: bounds.left,
          top: bounds.top,
          width: bounds.width,
        });
      }
      place();
      window.addEventListener("resize", place);
      return () => window.removeEventListener("resize", place);
    }, []);

    function choose(skill: RegisteredSkill) {
      onSelect(skill);
      onDismiss();
    }

    const left = position?.left ?? 0;
    const top = position?.top ?? 0;
    const width = position?.width ?? 380;

    return (
      <div
        class="theme-menu-surface codex-command-palette fixed z-40 flex flex-col overflow-hidden rounded-lg border border-line bg-raised shadow-2xl"
        style={{
          left: `${left}px`,
          top: `${top}px`,
          width: `${width}px`,
          visibility: position ? "visible" : "hidden",
        }}
        role="listbox"
        aria-label="Commands"
        data-testid="command-palette"
      >
        <div class="flex flex-none items-center gap-2 border-b border-line bg-surface px-2.5 py-2">
          <Code class="h-3.5 w-3.5 flex-none text-accent-blue" aria-hidden="true" />
          <span class="min-w-0 flex-1 truncate text-[12.5px] text-ink-300">
            {query ? `Commands for "/${query}"` : "Commands (type to filter)"}
          </span>
          <span class="flex-none rounded bg-tint-strong px-1.5 py-0.5 text-[10px] text-ink-400">
            {loading ? "…" : results.length}
          </span>
        </div>

        <div ref={listRef} class="min-h-0 max-h-80 flex-1 overflow-y-auto py-1">
          {error ? (
            <div class="px-3 py-3 text-[12px] text-accent-red">{error}</div>
          ) : loading ? (
            <div class="px-3 py-3 text-[12px] text-ink-400">Loading commands...</div>
          ) : results.length === 0 ? (
            <div class="px-3 py-3 text-[12px] text-ink-400">
              {skills.length === 0 ? "No commands registered" : "No matching commands"}
            </div>
          ) : (
            results.slice(0, MAX_VISIBLE).map((skill, index) => (
              <button
                key={`${skill.source || "skill"}:${skill.command || skill.name}`}
                type="button"
                data-index={index}
                onClick={() => choose(skill)}
                onMouseEnter={() => setHighlighted(index)}
                role="option"
                aria-selected={highlighted === index}
                class={`block w-full px-3 py-2 text-left focus:outline-none ${
                  highlighted === index ? "bg-accent-blue/[0.12] text-ink-100" : "text-ink-200 hover:bg-tint-strong"
                }`}
              >
                <div class="flex items-center gap-2 min-w-0">
                  <span class="truncate text-[13px] font-medium">
                    {skill.command || skill.name}
                  </span>
                  {skill.source && (
                    <span class="flex-none rounded bg-tint-strong px-1.5 py-0.5 text-[10px] uppercase text-ink-400">
                      {skill.source}
                    </span>
                  )}
                </div>
                {skill.description && (
                  <div class="mt-0.5 max-h-8 overflow-hidden text-[12px] leading-4 text-ink-400">
                    {skill.description}
                  </div>
                )}
              </button>
            ))
          )}
        </div>
      </div>
    );
  }
);
