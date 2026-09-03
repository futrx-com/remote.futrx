import { useDeleteProjectConfirmation } from "../../state/hooks/projects/useDeleteProjectConfirmation";
import { AlertCircle, Loader, X } from "../primitives/icons";

export function DeleteProjectModal({
  open,
  projectName,
  onClose,
  onDelete,
}: {
  open: boolean;
  projectName: string;
  onClose: () => void;
  onDelete: () => Promise<void>;
}) {
  const confirmationState = useDeleteProjectConfirmation({
    open,
    projectName,
    onClose,
    onDelete,
  });

  if (!open) return null;

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-8">
      <div
        class="absolute inset-0 bg-black/55 backdrop-blur-[3px] modal-backdrop-fade"
        onClick={confirmationState.close}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-project-title"
        class="theme-menu-surface modal-card-pop relative w-full max-w-[480px] overflow-hidden rounded-[14px] border border-line bg-ink-800 text-ink-50 shadow-[0_24px_64px_rgba(0,0,0,.6)]"
      >
        <div class="flex items-start justify-between gap-4 px-5 pb-3.5 pt-[18px]">
          <div class="flex flex-col gap-[3px]">
            <div id="delete-project-title" class="text-[15px] font-semibold tracking-[-0.01em]">
              Delete project
            </div>
            <div class="text-[12.5px] text-ink-300">
              This action cannot be undone.
            </div>
          </div>
          <button
            type="button"
            onClick={confirmationState.close}
            disabled={confirmationState.deleting}
            aria-label="Close"
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded-[7px] text-ink-300 transition-colors hover:bg-tint hover:text-ink-100 disabled:opacity-45"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        <div class="flex flex-col gap-4 border-t border-line p-5">
          <div class="rounded-[10px] border border-accent-red/25 bg-accent-red/[0.07] px-3.5 py-3 text-[13px] leading-5 text-ink-200">
            The <span class="font-mono text-ink-50">{projectName}</span> container, project settings, and associated chats will be permanently removed.
          </div>

          <div class="flex flex-col gap-[7px]">
            <label for="delete-project-confirmation" class="text-xs text-ink-300">
              Type <span class="font-mono text-ink-100">{projectName}</span> to confirm
            </label>
            <input
              id="delete-project-confirmation"
              ref={confirmationState.inputRef}
              value={confirmationState.confirmation}
              onInput={(event) => {
                confirmationState.updateConfirmation((event.target as HTMLInputElement).value);
              }}
              onKeyDown={(event) => {
                if (event.key === "Enter") void confirmationState.submit();
              }}
              autocomplete="off"
              spellcheck={false}
              disabled={confirmationState.deleting}
              class="theme-submenu-surface w-full rounded-[9px] border border-line-strong bg-raised px-3 py-2.5 font-mono text-sm text-ink-100 outline-none transition-[border-color,box-shadow] duration-150 focus:border-accent-red/60 focus:shadow-[0_0_0_3px_rgba(255,123,114,.12)]"
            />
          </div>

          {confirmationState.deleteError && (
            <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
              <AlertCircle class="mt-0.5 h-4 w-4 flex-none text-accent-red" />
              <div class="break-words text-accent-red">{confirmationState.deleteError}</div>
            </div>
          )}
        </div>

        <div class="flex items-center justify-end gap-2 border-t border-line bg-tint px-5 py-3.5">
          <button
            type="button"
            onClick={confirmationState.close}
            disabled={confirmationState.deleting}
            class="rounded-lg border border-line-strong px-3.5 py-2 text-[13px] text-ink-200 transition-colors hover:bg-tint hover:text-ink-100 disabled:opacity-45"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void confirmationState.submit()}
            disabled={!confirmationState.isConfirmed || confirmationState.deleting}
            class="inline-flex items-center gap-[7px] rounded-lg border border-accent-red/40 bg-accent-red px-[15px] py-2 text-[13px] font-semibold text-ink-900 transition-colors hover:bg-accent-red/90 disabled:cursor-not-allowed disabled:border-line disabled:bg-tint-strong disabled:text-ink-400"
          >
            {confirmationState.deleting && <Loader class="h-3.5 w-3.5 animate-spin" />}
            {confirmationState.deleting ? "Deleting…" : "Delete project"}
          </button>
        </div>
      </div>
    </div>
  );
}
