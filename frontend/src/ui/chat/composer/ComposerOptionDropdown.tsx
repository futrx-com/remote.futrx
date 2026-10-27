import type { JSX } from "preact";
import { useEffect, useLayoutEffect, useRef, useState } from "preact/hooks";
import { Check, ChevronDown } from "../../primitives/icons";

export interface ComposerOption<T extends string> {
  value: T;
  label: string;
  description?: string;
  details?: readonly string[];
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
  const [expandedOption, setExpandedOption] = useState<T | null>(null);
  const [menuLeft, setMenuLeft] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const selected = options.find((option) => option.value === value) || options[0];
  const hasDescriptions = options.some(
    (option) => option.description || option.details?.length,
  );

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
      const furthestLeft = viewportGutter;
      const furthestRight = Math.max(viewportGutter, window.innerWidth - viewportGutter - menuWidth);
      const preferredLeft = rootBounds.left;
      const clampedLeft = Math.min(Math.max(preferredLeft, furthestLeft), furthestRight);
      setMenuLeft(clampedLeft - rootBounds.left);
    }

    placeMenuWithinViewport();
    window.addEventListener("resize", placeMenuWithinViewport);
    return () => window.removeEventListener("resize", placeMenuWithinViewport);
  }, [open]);

  function pick(nextValue: T) {
    setOpen(false);
    setExpandedOption(null);
    if (nextValue !== value) onChange(nextValue);
  }

  return (
    <div ref={rootRef} class="codex-option-control relative flex-none">
      <button
        type="button"
        onClick={() => {
          setMenuLeft(0);
          setOpen((current) => !current);
        }}
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
          class={`theme-menu-surface menu-pop-up absolute bottom-full z-40 mb-1.5 ${hasDescriptions ? "w-[min(20rem,calc(100vw-1.5rem))]" : "w-[min(11rem,calc(100vw-1.5rem))]"} rounded-card border border-line bg-raised p-1 shadow-pop`}
          style={{ left: `${menuLeft}px` }}
          role="listbox"
        >
          {options.map((option) => {
            const active = option.value === value;
            const hasMore = Boolean(option.description || option.details?.length);
            const expanded = expandedOption === option.value;
            const detailsId = `${label}-${option.value || "auto"}-details`
              .toLowerCase()
              .replace(/[^a-z0-9-]+/g, "-");
            return (
              <div
                key={option.value || "auto"}
                class={`rounded-control transition-colors ${active ? "bg-tint-active text-ink-50" : "text-ink-200 hover:bg-tint-strong"}`}
              >
                <div class="flex items-center">
                  <button
                    type="button"
                    onClick={() => pick(option.value)}
                    class="flex min-w-0 flex-1 items-center justify-between gap-2 px-2.5 py-2 text-left"
                    role="option"
                    aria-selected={active}
                  >
                    <span class="min-w-0">
                      <span class={`block truncate text-[12.5px] ${active ? "font-medium" : ""}`}>
                        {option.label}
                      </span>
                      {option.description && (
                        <span class="mt-0.5 block text-[11px] font-normal leading-4 text-ink-400">
                          {option.description}
                        </span>
                      )}
                    </span>
                    {active && <Check class="h-3 w-3 flex-none text-accent-blue" />}
                  </button>
                  {hasMore && (
                    <button
                      type="button"
                      onClick={() => setExpandedOption(expanded ? null : option.value)}
                      class="mr-1.5 inline-flex h-6 w-6 flex-none items-center justify-center rounded-control text-ink-400 transition-colors hover:bg-tint-strong hover:text-ink-100"
                      title={expanded ? `Hide ${option.label} details` : `Read more about ${option.label}`}
                      aria-label={expanded ? `Hide ${option.label} details` : `Read more about ${option.label}`}
                      aria-expanded={expanded}
                      aria-controls={detailsId}
                    >
                      <ChevronDown
                        class={`h-3.5 w-3.5 transition-transform duration-150 motion-reduce:transition-none ${expanded ? "rotate-180" : ""}`}
                        aria-hidden="true"
                      />
                    </button>
                  )}
                </div>
                {expanded && (
                  <div id={detailsId} class="px-2.5 pb-2">
                    {option.details && option.details.length > 0 && (
                      <ul class="space-y-0.5 text-[11px] font-normal leading-4 text-ink-400">
                        {option.details.map((detail) => (
                          <li key={detail} class="flex gap-1.5">
                            <span class="flex-none text-ink-500" aria-hidden="true">•</span>
                            <span>{detail}</span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
