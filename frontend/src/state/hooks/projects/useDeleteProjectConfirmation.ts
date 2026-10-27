import { useEffect, useRef, useState } from "preact/hooks";
import { useDismissShortcut } from "../shared/useDismissShortcut.ts";

export function useDeleteProjectConfirmation({
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
  const [confirmation, setConfirmation] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setConfirmation("");
    setDeleting(false);
    setDeleteError("");
    const timer = setTimeout(() => inputRef.current?.focus(), 60);
    return () => clearTimeout(timer);
  }, [open, projectName]);

  // close() reads `deleting`, and the handler is read through a ref, so an
  // Escape mid-delete sees the current flag without re-registering.
  useDismissShortcut(close, { enabled: open });

  const isConfirmed = confirmation === projectName;

  function close() {
    if (!deleting) onClose();
  }

  function updateConfirmation(value: string) {
    setConfirmation(value);
    setDeleteError("");
  }

  async function submit() {
    if (!isConfirmed || deleting) return;
    setDeleting(true);
    setDeleteError("");
    try {
      await onDelete();
      onClose();
    } catch (error) {
      setDeleteError("Delete failed: " + (error as Error).message);
      setDeleting(false);
    }
  }

  return {
    confirmation,
    deleting,
    deleteError,
    inputRef,
    isConfirmed,
    close,
    updateConfirmation,
    submit,
  };
}
