import type { SelectedSkill } from "../../../models/chat";
import { X } from "../../primitives/icons";

export function SelectedSkillChips({
  skills,
  onRemove,
}: {
  skills: SelectedSkill[];
  onRemove: (skill: SelectedSkill) => void;
}) {
  if (skills.length === 0) return null;

  return (
    <div class="px-3 pt-1">
      <div class="flex min-w-0 flex-wrap items-center gap-1.5">
        <span class="text-[11px] font-medium text-ink-500">Skills</span>
        {skills.map((skill) => (
          <span
            key={`${skill.provider || "skill"}:${skill.source || ""}:${skill.command || skill.name}`}
            class="inline-flex h-7 max-w-full items-center gap-1.5 rounded-md bg-tint px-2 text-[12px] text-ink-200"
            title={skillTitle(skill)}
          >
            <span class="truncate">{skill.name || skill.command}</span>
            {skill.source && (
              <span class="hidden rounded bg-tint-strong px-1 py-0.5 text-[9px] uppercase text-ink-500 sm:inline">
                {skill.source}
              </span>
            )}
            <button
              type="button"
              onClick={() => onRemove(skill)}
              class="inline-flex h-5 w-5 flex-none items-center justify-center rounded text-ink-500 hover:bg-tint-strong hover:text-ink-100"
              title={`Remove ${skill.name || skill.command}`}
              aria-label={`Remove ${skill.name || skill.command}`}
            >
              <X class="h-3 w-3" />
            </button>
          </span>
        ))}
      </div>
    </div>
  );
}

function skillTitle(skill: SelectedSkill) {
  const command = skill.command || skill.name;
  return `${skill.name || command}${command ? ` (${command})` : ""}`;
}
