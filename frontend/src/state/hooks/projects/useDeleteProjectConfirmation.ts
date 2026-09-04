import { useEffect, useRef, useState } from "preact/hooks";

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

  // close() reads `deleting` and `onClose`, so the listener has to reach the
  // current one; keying the effect on those instead would re-register it
  // whenever either changed. The ref keeps the handler current while the
  // listener is registered once per open.
  const closeRef = useRef(close);
  closeRef.current = close;

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeRef.current();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

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
