import type { ComponentChildren } from "preact";
import { ConfirmContext, useConfirmController } from "../../state/context/ConfirmContext";
import { ConfirmDialog } from "../../ui/primitives/ConfirmDialog";

/**
 * Binds the confirm state machine to the dialog that draws it. The two halves
 * meet here, in the app layer, so the state layer never reaches into `ui/`.
 */
export function ConfirmProvider({ children }: { children: ComponentChildren }) {
  const { confirm, options, pending, error, cancel, accept } = useConfirmController();

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <ConfirmDialog
        open={!!options}
        title={options?.title || ""}
        description={options?.description}
        confirmLabel={options?.confirmLabel || "Confirm"}
        cancelLabel={options?.cancelLabel}
        pendingLabel={options?.pendingLabel}
        tone={options?.tone}
        pending={pending}
        error={error}
        onCancel={cancel}
        onConfirm={() => void accept()}
      >
        {options?.message}
      </ConfirmDialog>
    </ConfirmContext.Provider>
  );
}
