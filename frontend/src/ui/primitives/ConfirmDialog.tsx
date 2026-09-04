import type { ComponentChildren } from "preact";
import { useEffect, useRef } from "preact/hooks";
import type { ConfirmTone } from "../../state/context/ConfirmContext";
import { AlertCircle, Loader, X } from "./icons";

/**
 * The app-wide replacement for window.confirm(): same card chrome as the
 * create/delete project modals, so a destructive prompt never drops the user
 * back to the browser's own dialog.
 */
export function ConfirmDialog({
  open,
  title,
  description,
  children,
  confirmLabel,
  cancelLabel = "Cancel",
  pendingLabel,
  tone = "danger",
  pending = false,
  error = "",
  onCancel,
  onConfirm,
}: {
  open: boolean;
  title: string;
  description?: string;
  children?: ComponentChildren;
  confirmLabel: string;
  cancelLabel?: string;
  pendingLabel?: string;
  tone?: ConfirmTone;
  pending?: boolean;
  error?: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const confirmRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    // Focus once the pop animation has started so the browser doesn't scroll a
    // half-positioned card into view.
    const timer = setTimeout(() => confirmRef.current?.focus(), 60);
    return () => clearTimeout(timer);
  }, [open, title]);

  // The handler reads `pending` and `onCancel`, so it is held in a ref rather
  // than keyed on: an Escape mid-confirmation must see the current pending flag.
  const cancelOnEscapeRef = useRef(() => {});
  cancelOnEscapeRef.current = () => {
    if (!pending) onCancel();
  };

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") cancelOnEscapeRef.current();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  if (!open) return null;

  const danger = tone === "danger";

  return (
    <div class="fixed inset-0 z-[60] flex items-center justify-center p-4 sm:p-8">
      <div
        class="absolute inset-0 bg-black/55 backdrop-blur-[3px] modal-backdrop-fade"
        onClick={() => {
          if (!pending) onCancel();
        }}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        class="theme-menu-surface modal-card-pop relative w-full max-w-[440px] overflow-hidden rounded-[14px] border border-line bg-ink-800 text-ink-50 shadow-[0_24px_64px_rgba(0,0,0,.6)]"
      >
        <div class="flex items-start justify-between gap-4 px-5 pb-3.5 pt-[18px]">
          <div class="flex flex-col gap-[3px]">
            <div id="confirm-dialog-title" class="text-[15px] font-semibold tracking-[-0.01em]">
              {title}
            </div>
            {description && <div class="text-[12.5px] text-ink-300">{description}</div>}
          </div>
          <button
            type="button"
            onClick={onCancel}
            disabled={pending}
            aria-label="Close"
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded-[7px] text-ink-300 transition-colors hover:bg-tint hover:text-ink-100 disabled:opacity-45"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="flex flex-col gap-4 border-t border-line p-5">
          <div
            class={`rounded-[10px] border px-3.5 py-3 text-[13px] leading-5 text-ink-200 ${
              danger
                ? "border-accent-red/25 bg-accent-red/[0.07]"
                : "border-line-strong bg-tint"
            }`}
          >
            {children}
          </div>

          {error && (
            <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
              <AlertCircle class="mt-0.5 h-4 w-4 flex-none text-accent-red" />
              <div class="break-words text-accent-red">{error}</div>
            </div>
          )}
        </div>

        <div class="flex items-center justify-end gap-2 border-t border-line bg-tint px-5 py-3.5">
          <button
            type="button"
            onClick={onCancel}
            disabled={pending}
            class="rounded-lg border border-line-strong px-3.5 py-2 text-[13px] text-ink-200 transition-colors hover:bg-tint hover:text-ink-100 disabled:opacity-45"
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            ref={confirmRef}
            onClick={onConfirm}
            disabled={pending}
            class={`inline-flex items-center gap-[7px] rounded-lg border px-[15px] py-2 text-[13px] font-semibold transition-colors disabled:cursor-not-allowed disabled:border-line disabled:bg-tint-strong disabled:text-ink-400 ${
              danger
                ? "border-accent-red/40 bg-accent-red text-ink-900 hover:bg-accent-red/90"
                : "border-accent-blue/40 bg-accent-blue text-on-accent hover:bg-accent-blue/90"
            }`}
          >
            {pending && <Loader class="h-3.5 w-3.5 animate-spin" />}
            {pending ? pendingLabel || "Working…" : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
