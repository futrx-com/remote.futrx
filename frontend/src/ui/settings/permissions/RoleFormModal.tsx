import { useState } from "preact/hooks";
import type { RbacDefinition, RbacRole, RbacRoleInput, RbacRoleRule } from "../../../models/rbac";
import { permissionForms } from "../../../state/hooks/permissions/permissionForms";
import { Loader, X } from "../../primitives/icons";
import { PermissionModalShell } from "./PermissionModalShell";
import { SELECT_CLASS } from "./PermissionFormFields";

const FIELD_CLASS =
  "theme-submenu-surface w-full rounded-[9px] border border-line-strong bg-raised px-3 py-2.5 text-sm text-ink-100 outline-none transition-[border-color,box-shadow] duration-150";

// `role` present means edit; absent means create.
export function RoleFormModal({
  role,
  roles,
  definitions,
  onSubmit,
  onClose,
}: {
  role?: RbacRole;
  roles: RbacRole[];
  definitions: RbacDefinition[];
  onSubmit: (input: RbacRoleInput) => Promise<void>;
  onClose: () => void;
}) {
  const [name, setName] = useState(role?.name ?? "");
  const [description, setDescription] = useState(role?.description ?? "");
  const [rules, setRules] = useState<RbacRoleRule[]>(role?.rules ?? []);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const updateRule = (index: number, patch: Partial<RbacRoleRule>) =>
    setRules((current) => current.map((rule, i) => (i === index ? { ...rule, ...patch } : rule)));

  const submit = async () => {
    const result = permissionForms.validateRole({ name, description, rules }, roles, definitions, role?.id);
    if (!result.ok) {
      setErr(result.message);
      return;
    }
    setErr(null);
    setBusy(true);
    try {
      await onSubmit(result.input);
      onClose();
    } catch (cause) {
      setErr((cause as Error).message);
      setBusy(false);
    }
  };

  return (
    <PermissionModalShell
      titleId="role-form-title"
      title={role ? "Edit role" : "Create role"}
      subtitle="A role bundles allow and deny rules that can be bound to users."
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
            disabled={busy}
            class="inline-flex items-center gap-[7px] rounded-lg border border-accent-blue/40 bg-accent-blue px-[15px] py-2 text-[13px] font-semibold text-on-accent transition-colors hover:bg-accent-blue/90 disabled:cursor-not-allowed disabled:border-line disabled:bg-tint-strong disabled:text-ink-400"
          >
            {busy && <Loader class="h-3.5 w-3.5 animate-spin" />}
            {busy ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      <div class="flex flex-col gap-[7px]">
        <label for="role-name" class="text-xs uppercase tracking-[0.08em] text-ink-300">Name</label>
        <input
          id="role-name"
          value={name}
          onInput={(e) => setName((e.target as HTMLInputElement).value)}
          autocomplete="off"
          spellcheck={false}
          disabled={busy}
          class={FIELD_CLASS}
        />
      </div>
      <div class="flex flex-col gap-[7px]">
        <label for="role-description" class="text-xs uppercase tracking-[0.08em] text-ink-300">Description</label>
        <textarea
          id="role-description"
          value={description}
          onInput={(e) => setDescription((e.target as HTMLTextAreaElement).value)}
          rows={2}
          disabled={busy}
          class={FIELD_CLASS}
        />
      </div>
      <div class="flex flex-col gap-[7px]">
        <div class="flex items-center justify-between">
          <div class="text-xs uppercase tracking-[0.08em] text-ink-300">Rules</div>
          <button
            type="button"
            onClick={() => setRules((current) => [...current, { permission: "", effect: "allow" }])}
            disabled={busy}
            class="h-7 px-2 rounded text-[11px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong disabled:opacity-50"
          >
            Add rule
          </button>
        </div>
        {rules.map((rule, index) => (
          <div key={index} class="grid grid-cols-[1fr_auto_auto] gap-2 items-center">
            <select
              value={rule.permission}
              onChange={(e) => updateRule(index, { permission: (e.target as HTMLSelectElement).value })}
              disabled={busy}
              class={`${SELECT_CLASS} font-mono`}
              aria-label="Permission"
            >
              <option value="">Choose permission…</option>
              {definitions.map((definition) => (
                <option key={definition.key} value={definition.key}>{definition.key}</option>
              ))}
            </select>
            <select
              value={rule.effect}
              onChange={(e) =>
                updateRule(index, { effect: (e.target as HTMLSelectElement).value as RbacRoleRule["effect"] })
              }
              disabled={busy}
              class={SELECT_CLASS}
              aria-label="Effect"
            >
              <option value="allow">allow</option>
              <option value="deny">deny</option>
            </select>
            <button
              type="button"
              onClick={() => setRules((current) => current.filter((_, i) => i !== index))}
              disabled={busy}
              class="h-7 w-7 rounded text-ink-300 hover:text-accent-red hover:bg-tint-strong grid place-items-center disabled:opacity-50"
              aria-label="Remove rule"
              title="Remove"
            >
              <X class="w-3.5 h-3.5" />
            </button>
          </div>
        ))}
      </div>
      {err && <div class="text-xs text-accent-red">{err}</div>}
    </PermissionModalShell>
  );
}
