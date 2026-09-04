import type { RegisteredSkill } from "../../../models/skill";

export function SlashCommandMenu({
  items,
  highlight,
  loading,
  error,
  query,
  onChoose,
  onHighlight,
}: {
  items: RegisteredSkill[];
  highlight: number;
  loading: boolean;
  error: string;
  query: string;
  onChoose: (skill: RegisteredSkill) => void;
  onHighlight: (index: number) => void;
}) {
  return (
    <div
      class="theme-menu-surface absolute left-0 right-0 bottom-full z-40 mb-2
             overflow-hidden rounded-xl border border-white/10 bg-[#14161d] shadow-2xl"
      role="listbox"
      aria-label="Commands"
    >
      <div class="max-h-[320px] overflow-y-auto py-1">
        {error ? (
          <div class="px-3 py-3 text-[12px] text-red-300">{error}</div>
        ) : loading ? (
          <div class="px-3 py-3 text-[12px] text-ink-400">Loading commands...</div>
        ) : items.length === 0 ? (
          <div class="px-3 py-3 text-[12px] text-ink-400">
            {query ? `No commands match “/${query}”` : "No commands available"}
          </div>
        ) : (
          items.map((skill, index) => {
            const command = skill.command || skill.name;
            const active = index === highlight;
            return (
              <button
                key={`${skill.source || "skill"}:${command}`}
                type="button"
                role="option"
                aria-selected={active}
                // Use mousedown so the choice lands before the textarea blurs.
                onMouseDown={(event) => {
                  event.preventDefault();
                  onChoose(skill);
                }}
                onMouseEnter={() => onHighlight(index)}
                class={`flex w-full items-center gap-3 px-3 py-2.5 text-left focus:outline-none
                        ${active ? "bg-white/[0.08]" : "hover:bg-white/[0.05]"}`}
              >
                <span class="flex min-w-0 flex-none items-center gap-2">
                  <span class="truncate text-[13px] font-medium text-ink-100">/{command}</span>
                  {skill.source && (
                    <span class="flex-none rounded bg-white/[0.08] px-1.5 py-0.5 text-[10px] uppercase text-ink-400">
                      {skill.source}
                    </span>
                  )}
                </span>
                {skill.description && (
                  <span class="min-w-0 flex-1 truncate text-right text-[12px] text-ink-400">
                    {skill.description}
                  </span>
                )}
              </button>
            );
          })
        )}
      </div>
    </div>
  );
}
