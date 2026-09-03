import { Check } from "../../../primitives/icons";

export function QuestionOption({
  label,
  description,
  active,
  multi,
  onClick,
}: {
  label: string;
  description?: string;
  active: boolean;
  multi: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      class={`text-left rounded-md border px-3 py-2.5 min-h-12 transition-colors
              ${active
                ? "border-accent-blue bg-accent-blue/15"
                : "border-line bg-tint hover:bg-tint-strong"}`}
    >
      <div class="flex items-start gap-2">
        <div class={`flex-none mt-0.5 w-4 h-4 ${multi ? "rounded-sm" : "rounded-full"}
                     border ${active ? "bg-accent-blue border-accent-blue" : "border-ink-300"}
                     grid place-items-center`}>
          {active && <Check class="w-3 h-3 text-white" />}
        </div>
        <div class="flex-1 min-w-0">
          <div class={`text-[13px] font-medium ${active ? "text-accent-blue" : "text-ink-100"}`}>
            {label}
          </div>
          {description && (
            <div class="text-[11.5px] text-ink-300 mt-0.5 leading-snug">
              {description}
            </div>
          )}
        </div>
      </div>
    </button>
  );
}
