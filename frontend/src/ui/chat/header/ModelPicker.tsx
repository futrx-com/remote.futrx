import type { RefObject } from "preact";
import { Check, ChevronDown } from "../../primitives/icons";

export function ModelPicker({
  modelRef,
  open,
  model,
  streaming,
  options,
  displayLabel,
  onToggle,
  onPick,
}: {
  modelRef: RefObject<HTMLDivElement>;
  open: boolean;
  model?: string;
  streaming: boolean;
  options: Array<{ value: string; label: string; sub: string }>;
  displayLabel: (model?: string) => string;
  onToggle: () => void;
  onPick: (model: string) => void;
}) {
  return (
    <div ref={modelRef} class="relative flex-none">
      <button
        type="button"
        onClick={onToggle}
        class="inline-flex h-8 items-center justify-center gap-1.5 rounded-control px-2.5 text-[13px]
               font-medium text-ink-200 transition-colors hover:bg-tint-strong hover:text-ink-50
               disabled:opacity-50 sm:px-3"
        disabled={streaming}
        title={streaming ? "Cannot change model while streaming" : "Switch model"}
      >
        <span>{displayLabel(model)}</span>
        <ChevronDown class="h-3 w-3 text-ink-400" />
      </button>
      {open && (
        <div class="theme-menu-surface menu-pop absolute right-0 top-full z-40 mt-1.5 w-[230px]
                    overflow-hidden rounded-card border border-line bg-raised p-1 shadow-pop">
          {options.map((option) => {
            const active = (model || "") === option.value ||
              (option.value !== "" && displayLabel(model).toLowerCase() === option.value);
            return (
              <button
                key={option.value}
                type="button"
                onClick={() => onPick(option.value)}
                class={`flex w-full items-center justify-between gap-3 rounded-control px-2.5 py-2 text-left transition-colors
                        ${active ? "bg-tint-active" : "hover:bg-tint-strong"}`}
              >
                <span>
                  <span class={`block text-[13px] ${active ? "font-medium text-ink-50" : "text-ink-100"}`}>{option.label}</span>
                  <span class="block text-[11.5px] text-ink-400">{option.sub}</span>
                </span>
                {active && <Check class="h-3.5 w-3.5 flex-none text-accent-blue" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
