import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "preact/hooks";
import type { ChatProvider } from "../../../models/chat";
import type { RegisteredSkill } from "../../../models/skill";
import { useAvailableSkills } from "../../../state/hooks/chat/useAvailableSkills";
import { ChevronDown, Code, Search } from "../../primitives/icons";

const MENU_MAX_WIDTH = 460;
const MENU_MAX_HEIGHT = 360;
const MENU_MIN_HEIGHT = 180;
const MENU_GAP = 8;

interface MenuStyle {
  left: number;
  bottom: number;
  width: number;
  maxHeight: number;
}

export function SkillPicker({
  provider,
  providerLabel,
  projectId,
  selectedCount,
  onSelect,
}: {
  provider: ChatProvider;
  providerLabel: string;
  projectId?: string;
  selectedCount: number;
  onSelect: (skill: RegisteredSkill) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [menuStyle, setMenuStyle] = useState<MenuStyle | null>(null);
  const { skills, loading, error } = useAvailableSkills(provider, projectId);
  const rootRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setOpen(false);
    setQuery("");
  }, [provider, projectId]);

  useEffect(() => {
    if (!open) return;
    const id = window.setTimeout(() => searchRef.current?.focus(), 0);
    function closeOnOutsideClick(event: MouseEvent) {
      const target = event.target as Node | null;
      if (target && !rootRef.current?.contains(target)) setOpen(false);
    }
    window.addEventListener("mousedown", closeOnOutsideClick);
    return () => {
      window.clearTimeout(id);
      window.removeEventListener("mousedown", closeOnOutsideClick);
    };
  }, [open]);

  // The composer lives inside an `overflow-hidden` thread column, so an absolutely
  // positioned panel wider than the trigger gets clipped. Pin it to the viewport and
  // keep it inside the composer card instead.
  useLayoutEffect(() => {
    if (!open) {
      setMenuStyle(null);
      return;
    }
    function place() {
      const root = rootRef.current;
      if (!root) return;
      const trigger = root.getBoundingClientRect();
      const bounds = (root.closest(".codex-composer-card") ?? document.documentElement)
        .getBoundingClientRect();
      const width = Math.min(MENU_MAX_WIDTH, bounds.width);
      const left = Math.min(
        Math.max(trigger.right - width, bounds.left),
        Math.max(bounds.right - width, bounds.left)
      );
      setMenuStyle({
        left,
        width,
        bottom: window.innerHeight - trigger.top + MENU_GAP,
        maxHeight: Math.max(Math.min(MENU_MAX_HEIGHT, trigger.top - MENU_GAP * 2), MENU_MIN_HEIGHT),
      });
    }
    place();
    window.addEventListener("resize", place);
    return () => window.removeEventListener("resize", place);
  }, [open]);

  const filteredSkills = useMemo(() => {
    const term = query.trim().toLowerCase();
    if (!term) return skills;
    return skills.filter((skill) =>
      `${skill.name} ${skill.description || ""} ${skill.source || ""}`.toLowerCase().includes(term)
    );
  }, [query, skills]);

  function choose(skill: RegisteredSkill) {
    onSelect(skill);
    setOpen(false);
    setQuery("");
  }

  return (
    <div ref={rootRef} class="codex-skill-control-root relative w-[130px] flex-none sm:w-[148px]">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        class={`codex-skill-control flex h-7 w-full items-center justify-between gap-1.5 rounded-md px-2 text-left transition
                ${open ? "bg-accent-blue/[0.14] text-accent-blue" : "bg-accent-blue/[0.08] text-ink-100 hover:bg-accent-blue/[0.12]"}`}
        aria-haspopup="listbox"
        aria-expanded={open}
        title={`${providerLabel} skills`}
      >
        <span class="flex min-w-0 items-center gap-1.5">
          <Code class="h-3 w-3 flex-none" />
          <span class="truncate text-[11.5px] font-semibold">Skills</span>
        </span>
        <span class="inline-flex flex-none items-center gap-1">
          <span class="rounded bg-tint-strong px-1 py-0.5 text-[10px] leading-none text-ink-300">
            {selectedCount > 0 ? selectedCount : loading ? "..." : skills.length}
          </span>
          <ChevronDown class="h-3 w-3 flex-none" />
        </span>
      </button>

      {open && (
        <div
          class="theme-menu-surface fixed z-40 flex flex-col rounded-lg border border-line bg-raised shadow-2xl overflow-hidden"
          style={{
            left: `${menuStyle?.left ?? 0}px`,
            bottom: `${menuStyle?.bottom ?? 0}px`,
            width: `${menuStyle?.width ?? MENU_MAX_WIDTH}px`,
            maxHeight: `${menuStyle?.maxHeight ?? MENU_MAX_HEIGHT}px`,
            visibility: menuStyle ? "visible" : "hidden",
          }}
        >
          <div class="flex-none p-2 border-b border-line bg-surface">
            <div class="h-9 rounded-md bg-inset border border-line px-2.5 flex items-center gap-2">
              <Search class="w-3.5 h-3.5 text-ink-400 flex-none" />
              <input
                ref={searchRef}
                value={query}
                onInput={(event) => setQuery((event.currentTarget as HTMLInputElement).value)}
                class="min-w-0 flex-1 bg-transparent text-[13px] text-ink-100 placeholder:text-ink-500 focus:outline-none"
                placeholder={`Search ${providerLabel} skills`}
              />
            </div>
          </div>

          <div class="min-h-0 flex-1 overflow-y-auto py-1">
            {error ? (
              <div class="px-3 py-3 text-[12px] text-accent-red">{error}</div>
            ) : loading ? (
              <div class="px-3 py-3 text-[12px] text-ink-400">Loading skills...</div>
            ) : filteredSkills.length === 0 ? (
              <div class="px-3 py-3 text-[12px] text-ink-400">
                {skills.length === 0 ? `No ${providerLabel} skills registered` : "No matching skills"}
              </div>
            ) : (
              filteredSkills.map((skill) => (
                <button
                  key={`${skill.source || "skill"}:${skill.name}`}
                  type="button"
                  onClick={() => choose(skill)}
                  class="w-full text-left px-3 py-2.5 hover:bg-tint-strong focus:bg-tint-strong focus:outline-none"
                  role="option"
                >
                  <div class="flex items-center gap-2 min-w-0">
                    <span class="truncate text-[13px] font-medium text-ink-100">{skill.name}</span>
                    {skill.source && (
                      <span class="flex-none rounded bg-tint-strong px-1.5 py-0.5 text-[10px] uppercase text-ink-400">
                        {skill.source}
                      </span>
                    )}
                  </div>
                  {skill.description && (
                    <div class="mt-1 max-h-8 overflow-hidden text-[12px] leading-4 text-ink-400">
                      {skill.description}
                    </div>
                  )}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
