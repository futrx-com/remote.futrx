import type { ComponentChildren } from "preact";
import { createContext } from "preact";
import { useCallback, useContext, useRef, useState } from "preact/hooks";

export type ConfirmTone = "danger" | "neutral";

export interface ConfirmOptions {
  title: string;
  /** Sub-line under the title — the stakes, not a restatement of the question. */
  description?: string;
  /** What is about to happen, shown in the callout. */
  message: ComponentChildren;
  confirmLabel: string;
  cancelLabel?: string;
  tone?: ConfirmTone;
  /**
   * Optional work to run while the dialog stays open. A rejection is surfaced
   * inline instead of closing the dialog, so the user can retry or back out.
   */
  action?: () => Promise<unknown>;
  /** Button label while `action` is in flight. */
  pendingLabel?: string;
}

export type ConfirmFn = (options: ConfirmOptions) => Promise<boolean>;

interface ConfirmRequest {
  options: ConfirmOptions;
  resolve: (confirmed: boolean) => void;
}

export const ConfirmContext = createContext<ConfirmFn | null>(null);

/**
 * The confirm prompt as a state machine: it owns the pending request and the
 * promise every caller is waiting on, and knows nothing about how the question
 * is drawn. The app layer binds it to a dialog.
 */
export function useConfirmController() {
  //////////////////
  // Local State
  ///////////////////
  const [request, setRequest] = useState<ConfirmRequest | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  /////////////////
  // Refs
  ////////////////
  const pendingRef = useRef(false);

  ////////////////
  // Handlers
  ///////////////
  const confirm = useCallback<ConfirmFn>((options) => {
    return new Promise<boolean>((resolve) => {
      setError("");
      setPending(false);
      pendingRef.current = false;
      setRequest((previous) => {
        // A second prompt replaces the first rather than stacking cards; the
        // caller waiting on the displaced one is answered "not confirmed".
        previous?.resolve(false);
        return { options, resolve };
      });
    });
  }, []);

  function settle(current: ConfirmRequest, confirmed: boolean) {
    setRequest(null);
    setPending(false);
    pendingRef.current = false;
    setError("");
    current.resolve(confirmed);
  }

  function cancel() {
    if (!request || pendingRef.current) return;
    settle(request, false);
  }

  async function accept() {
    if (!request || pendingRef.current) return;
    const { action } = request.options;
    if (!action) {
      settle(request, true);
      return;
    }
    pendingRef.current = true;
    setPending(true);
    setError("");
    try {
      await action();
      settle(request, true);
    } catch (actionError) {
      pendingRef.current = false;
      setPending(false);
      setError((actionError as Error).message || "Something went wrong.");
    }
  }

  return { confirm, options: request?.options ?? null, pending, error, cancel, accept };
}

export function useConfirm(): ConfirmFn {
  const value = useContext(ConfirmContext);
  if (!value) throw new Error("useConfirm must be used inside ConfirmProvider");
  return value;
}
