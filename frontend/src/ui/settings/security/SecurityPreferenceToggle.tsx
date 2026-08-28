import { useState } from "preact/hooks";

export function SecurityPreferenceToggle({
  title,
  description,
  checked,
  disabled,
  disabledNote,
  onChange,
}: {
  title: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  disabledNote?: string;
  onChange: (checked: boolean) => Promise<void>;
}) {
  const [saving, setSaving] = useState(false);
  return (
    <section class="rounded-lg border border-white/10 bg-[#101318] p-3.5 flex items-start gap-3">
      <div class="flex-1 min-w-0">
        <div class="text-[14px] font-medium text-ink-50">{title}</div>
        <div class="text-[12.5px] text-ink-300 mt-0.5 leading-relaxed">{description}</div>
        {disabled && disabledNote && (
          <div class="text-[11.5px] text-ink-400 mt-1">{disabledNote}</div>
        )}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled || saving}
        onClick={async () => {
          setSaving(true);
          try {
            await onChange(!checked);
          } finally {
            setSaving(false);
          }
        }}
        class={`h-6 w-11 rounded-full flex-none transition-colors relative disabled:opacity-40 ${
          checked ? "bg-accent-blue" : "bg-white/15"
        }`}
      >
        <span
          class={`absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white transition-transform ${
            checked ? "translate-x-5" : ""
          }`}
        />
      </button>
    </section>
  );
}
