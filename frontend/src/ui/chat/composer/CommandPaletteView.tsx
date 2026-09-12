import { useEffect, useLayoutEffect, useRef, useState } from "preact/hooks";
import type { RegisteredSkill } from "../../../models/skill";
import { Code } from "../../primitives/icons";

const MAX_VISIBLE = 8;

interface PalettePosition {
  left: number;
  top: number;
  width: number;
}

export function CommandPaletteView({
  query,
  items,
  registeredCount,
  highlighted,
  loading,
  error,
  onChoose,
  onHighlight,
}: {
  query: string;
  items: RegisteredSkill[];
  registeredCount: number;
  highlighted: number;
  loading: boolean;
  error: string;
  onChoose: (skill: RegisteredSkill) => void;
  onHighlight: (index: number) => void;
}) {
  const [position, setPosition] = useState<PalettePosition | null>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (items.length === 0) return;
    listRef.current
      ?.querySelector<HTMLElement>(`[data-index="${highlighted}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [highlighted, items.length]);

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
          {loading ? "…" : items.length}
        </span>
      </div>

      <div ref={listRef} class="min-h-0 max-h-80 flex-1 overflow-y-auto py-1">
        {error ? (
          <div class="px-3 py-3 text-[12px] text-accent-red">{error}</div>
        ) : loading ? (
          <div class="px-3 py-3 text-[12px] text-ink-400">Loading commands...</div>
        ) : items.length === 0 ? (
          <div class="px-3 py-3 text-[12px] text-ink-400">
            {registeredCount === 0 ? "No commands registered" : "No matching commands"}
          </div>
        ) : (
          items.slice(0, MAX_VISIBLE).map((skill, index) => (
            <button
              key={`${skill.source || "skill"}:${skill.command || skill.name}`}
              type="button"
              data-index={index}
              onClick={() => onChoose(skill)}
              onMouseEnter={() => onHighlight(index)}
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
