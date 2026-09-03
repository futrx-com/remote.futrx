import { Check } from "../../../primitives/icons";

export function OtherAnswerOption({
  active,
  multi,
  value,
  onActivate,
  onChange,
}: {
  active: boolean;
  multi: boolean;
  value: string;
  onActivate: () => void;
  onChange: (value: string) => void;
}) {
  return (
    <button
      type="button"
      onClick={onActivate}
      class={`text-left rounded-md border px-3 py-2.5 min-h-12 transition-colors
              ${active
                ? "border-accent-blue bg-accent-blue/15"
                : "border-line border-dashed bg-tint hover:bg-tint-strong"} sm:col-span-2`}
    >
      <div class="flex items-start gap-2">
        <div class={`flex-none mt-0.5 w-4 h-4 ${multi ? "rounded-sm" : "rounded-full"}
                     border ${active ? "bg-accent-blue border-accent-blue" : "border-ink-300"}
                     grid place-items-center`}>
          {active && <Check class="w-3 h-3 text-white" />}
        </div>
        <div class="flex-1 min-w-0">
          <div class={`text-[13px] font-medium ${active ? "text-accent-blue" : "text-ink-200"}`}>
            Other (write your own)
          </div>
          {active && (
            <textarea
              autofocus
              rows={2}
              value={value}
              onInput={(event) => onChange((event.currentTarget as HTMLTextAreaElement).value)}
              onClick={(event) => event.stopPropagation()}
              placeholder="Your custom answer"
              class="mt-2 w-full resize-none bg-inset border border-line rounded-md
                     text-ink-100 text-[13px] px-2 py-1 focus:outline-none focus:border-accent-blue"
            />
          )}
        </div>
      </div>
    </button>
  );
}
