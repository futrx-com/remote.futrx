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

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

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
