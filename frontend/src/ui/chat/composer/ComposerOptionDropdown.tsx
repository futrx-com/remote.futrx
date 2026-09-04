import type { JSX } from "preact";
import { useEffect, useLayoutEffect, useRef, useState } from "preact/hooks";
import { Check, ChevronDown } from "../../primitives/icons";

export interface ComposerOption<T extends string> {
  value: T;
  label: string;
}

export function ComposerOptionDropdown<T extends string>({
  label,
  value,
  options,
  disabled = false,
  Icon,
  onChange,
}: {
  label: string;
  value: T;
  options: readonly ComposerOption<T>[];
  disabled?: boolean;
  Icon: (props: JSX.SVGAttributes<SVGSVGElement>) => JSX.Element;
  onChange: (value: T) => void;
}) {
  const [open, setOpen] = useState(false);
  const [menuAlignment, setMenuAlignment] = useState<"start" | "end">("start");
  const rootRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const selected = options.find((option) => option.value === value) || options[0];

  useEffect(() => {
    if (!open) return;
    function closeOnOutsideClick(event: MouseEvent) {
      const target = event.target as Node | null;
      if (target && !rootRef.current?.contains(target)) setOpen(false);
    }
    window.addEventListener("mousedown", closeOnOutsideClick);
    return () => window.removeEventListener("mousedown", closeOnOutsideClick);
  }, [open]);

  useLayoutEffect(() => {
    if (!open) return;

    function placeMenuWithinViewport() {
      const rootBounds = rootRef.current?.getBoundingClientRect();
      const menuWidth = menuRef.current?.offsetWidth;
      if (!rootBounds || !menuWidth) return;

      const viewportGutter = 12;
      const availableWidthAfterTrigger = window.innerWidth - viewportGutter - rootBounds.left;
      setMenuAlignment(menuWidth > availableWidthAfterTrigger ? "end" : "start");
    }

    placeMenuWithinViewport();
    window.addEventListener("resize", placeMenuWithinViewport);
    return () => window.removeEventListener("resize", placeMenuWithinViewport);
  }, [open]);

  function pick(nextValue: T) {
    setOpen(false);
    if (nextValue !== value) onChange(nextValue);
  }

  return (
    <div ref={rootRef} class="codex-option-control relative flex-none">
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        class={`inline-flex h-7 items-center gap-1.5 rounded-control px-2 text-[11.5px] transition-colors disabled:cursor-not-allowed disabled:opacity-50
                ${open ? "bg-tint-active text-ink-50" : "text-ink-300 hover:bg-tint-strong hover:text-ink-100"}`}
        disabled={disabled}
        title={`${label}: ${selected?.label || "Auto"}`}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        <span
          class="inline-flex h-4 w-4 flex-none items-center justify-center text-current opacity-60"
          title={label}
          aria-label={label}
        >
          <Icon class="h-3 w-3" />
        </span>
        <span class="sr-only">{label}</span>
        <span class="max-w-[7rem] truncate font-medium">{selected?.label || "Auto"}</span>
        <ChevronDown class="h-3 w-3 flex-none opacity-50" />
      </button>

      {open && (
        <div
          ref={menuRef}
          class={`theme-menu-surface menu-pop-up absolute bottom-full z-40 mb-1.5 w-[min(11rem,calc(100vw-1.5rem))] rounded-card border border-line bg-raised p-1 shadow-pop
                  ${menuAlignment === "end" ? "right-0" : "left-0"}`}
          role="listbox"
        >
          {options.map((option) => {
            const active = option.value === value;
            return (
              <button
                key={option.value || "auto"}
                type="button"
                onClick={() => pick(option.value)}
                class={`flex w-full items-center justify-between gap-2 rounded-control px-2.5 py-1.5 text-left text-[12.5px] transition-colors
                        ${active ? "bg-tint-active font-medium text-ink-50" : "text-ink-200 hover:bg-tint-strong"}`}
                role="option"
                aria-selected={active}
              >
                <span class="truncate">{option.label}</span>
                {active && <Check class="h-3 w-3 flex-none text-accent-blue" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
