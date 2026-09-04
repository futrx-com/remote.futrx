import type { AppearanceTheme } from "../../models/settings";
import { Check, Loader, Monitor, Moon, Sun } from "../primitives/icons";

const options: Array<{
  theme: AppearanceTheme;
  label: string;
  Icon: typeof Monitor;
}> = [
  { theme: "system", label: "System", Icon: Monitor },
  { theme: "dark", label: "Dark", Icon: Moon },
  { theme: "light", label: "Light", Icon: Sun },
];

export function AppearanceSettings({
  theme,
  loading,
  saving,
  error,
  onThemeChange,
}: {
  theme: AppearanceTheme;
  loading: boolean;
  saving: boolean;
  error: string | null;
  onThemeChange: (theme: AppearanceTheme) => void;
}) {
  return (
    <section class="overflow-hidden rounded-card border border-line bg-surface">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-line">
        <div class="mt-0.5 grid h-8 w-8 flex-none place-items-center rounded-control bg-tint text-ink-300">
          <Monitor class="h-4 w-4" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">Appearance</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">Theme preference</div>
        </div>
        {(loading || saving) && <Loader class="w-4 h-4 mt-2 text-ink-300 animate-spin" />}
      </header>

      <div class="p-4 space-y-3">
        <div
          class="segmented grid-cols-3"
          role="radiogroup"
          aria-label="Theme"
        >
          {options.map(({ theme: optionTheme, label, Icon }) => {
            const selected = optionTheme === theme;
            return (
              <button
                key={optionTheme}
                type="button"
                disabled={loading || saving}
                onClick={() => onThemeChange(optionTheme)}
                class="segmented-option disabled:cursor-wait"
                aria-checked={selected}
                role="radio"
              >
                <Icon class="w-4 h-4" />
                <span class="truncate">{label}</span>
              </button>
            );
          })}
        </div>

        <div class="min-h-5 text-[12px]">
          {error ? (
            <span class="text-accent-red">{error}</span>
          ) : saving ? (
            <span class="text-ink-300">Saving</span>
          ) : (
            <span class="inline-flex items-center gap-1 text-accent-green">
              <Check class="w-3.5 h-3.5" /> Saved
            </span>
          )}
        </div>
      </div>
    </section>
  );
}
