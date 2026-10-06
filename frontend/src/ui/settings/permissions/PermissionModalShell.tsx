import type { ComponentChildren } from "preact";
import { useDismissShortcut } from "../../../state/hooks/shared/useDismissShortcut";
import { X } from "../../primitives/icons";

export function PermissionModalShell({
  titleId,
  title,
  subtitle,
  busy,
  wide = false,
  onClose,
  footer,
  children,
}: {
  titleId: string;
  title: string;
  subtitle: string;
  busy: boolean;
  wide?: boolean;
  onClose: () => void;
  footer: ComponentChildren;
  children: ComponentChildren;
}) {
  const close = () => {
    if (!busy) onClose();
  };
  useDismissShortcut(close, { enabled: true });

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-8">
      <div class="absolute inset-0 bg-black/55 backdrop-blur-[3px] modal-backdrop-fade" onClick={close} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        class={`theme-menu-surface modal-card-pop relative w-full ${wide ? "max-w-[860px]" : "max-w-[480px]"} max-h-full overflow-y-auto rounded-[14px] border border-line bg-ink-800 text-ink-50 shadow-[0_24px_64px_rgba(0,0,0,.6)]`}
      >
        <div class="flex items-start justify-between gap-4 px-5 pb-3.5 pt-[18px]">
          <div class="flex flex-col gap-[3px]">
            <div id={titleId} class="text-[15px] font-semibold tracking-[-0.01em]">{title}</div>
            <div class="text-[12.5px] text-ink-300">{subtitle}</div>
          </div>
          <button
            type="button"
            onClick={close}
            disabled={busy}
            aria-label="Close"
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded-[7px] text-ink-300 transition-colors hover:bg-tint hover:text-ink-100 disabled:opacity-45"
          >
            <X class="h-4 w-4" />
          </button>
        </div>
        <div class="flex flex-col gap-3.5 border-t border-line p-5">{children}</div>
        <div class="flex items-center justify-end gap-2 border-t border-line bg-tint px-5 py-3.5">
          {footer}
        </div>
      </div>
    </div>
  );
}
