import { useState } from "preact/hooks";
import type { RbacRole } from "../../../models/rbac";
import { permissionForms } from "../../../state/hooks/permissions/permissionForms";
import { Loader } from "../../primitives/icons";
import { PermissionModalShell } from "./PermissionModalShell";

export function DeleteRoleModal({
  role,
  bindingCount,
  onDelete,
  onClose,
}: {
  role: RbacRole;
  bindingCount: number;
  onDelete: (unbind: boolean) => Promise<void>;
  onClose: () => void;
}) {
  const [typed, setTyped] = useState("");
  const [unbind, setUnbind] = useState(false);
  const [showUnbind, setShowUnbind] = useState(bindingCount > 0);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const canSubmit = typed === role.name && (!showUnbind || unbind);

  const submit = async () => {
    setErr(null);
    setBusy(true);
    try {
      await onDelete(unbind);
      onClose();
    } catch (cause) {
      if (permissionForms.isRoleStillBound(cause)) setShowUnbind(true);
      setErr(permissionForms.deleteRoleErrorMessage(cause));
      setBusy(false);
    }
  };

  return (
    <PermissionModalShell
      titleId="delete-role-title"
      title="Delete role"
      subtitle="This action cannot be undone."
      busy={busy}
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            class="rounded-lg border border-line-strong px-3.5 py-2 text-[13px] text-ink-200 transition-colors hover:bg-tint hover:text-ink-100 disabled:opacity-45"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void submit()}
            disabled={!canSubmit || busy}
            class="inline-flex items-center gap-[7px] rounded-lg border border-accent-red/40 bg-accent-red px-[15px] py-2 text-[13px] font-semibold text-ink-900 transition-colors hover:bg-accent-red/90 disabled:cursor-not-allowed disabled:border-line disabled:bg-tint-strong disabled:text-ink-400"
          >
            {busy && <Loader class="h-3.5 w-3.5 animate-spin" />}
            {busy ? "Removing…" : "Remove"}
          </button>
        </>
      }
    >
      <div class="rounded-[10px] border border-accent-red/25 bg-accent-red/[0.07] px-3.5 py-3 text-[13px] leading-5 text-ink-200">
        Role <span class="font-mono text-ink-50">{role.name}</span> will be deleted.
      </div>
      {showUnbind && (
        <label class="flex items-start gap-2 text-[13px] text-ink-200">
          <input
            type="checkbox"
            checked={unbind}
            onChange={(e) => setUnbind((e.target as HTMLInputElement).checked)}
            disabled={busy}
            class="mt-0.5"
          />
          <span>
            Also remove its bindings
            {bindingCount > 0 ? ` (${bindingCount})` : ""}
          </span>
        </label>
      )}
      <input
        value={typed}
        onInput={(e) => setTyped((e.target as HTMLInputElement).value)}
        placeholder={role.name}
        autocomplete="off"
        spellcheck={false}
        disabled={busy}
        class="theme-submenu-surface w-full rounded-[9px] border border-line-strong bg-raised px-3 py-2.5 font-mono text-sm text-ink-100 outline-none transition-[border-color,box-shadow] duration-150 focus:border-accent-red/60 focus:shadow-[0_0_0_3px_rgba(255,123,114,.12)]"
        aria-label={`Type ${role.name} to confirm`}
      />
      {err && <div class="text-xs text-accent-red">{err}</div>}
    </PermissionModalShell>
  );
}
