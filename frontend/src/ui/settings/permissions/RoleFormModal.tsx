import { useState } from "preact/hooks";
import type { RbacDefinition, RbacRole, RbacRoleInput, RbacRoleRule } from "../../../models/rbac";
import { permissionBranches } from "../../../state/hooks/permissions/permissionBranches";
import { permissionForms } from "../../../state/hooks/permissions/permissionForms";
import { Loader } from "../../primitives/icons";
import { BranchConfigurator } from "./BranchConfigurator";
import { BranchOverviewCard } from "./BranchOverviewCard";
import { PermissionModalShell } from "./PermissionModalShell";

const FIELD_CLASS =
  "theme-submenu-surface w-full rounded-[9px] border border-line-strong bg-raised px-3 py-2.5 text-sm text-ink-100 outline-none transition-[border-color,box-shadow] duration-150";

// `role` present means edit; absent means create.
export function RoleFormModal({
  role,
  roles,
  definitions,
  grantAll,
  onSubmit,
  onClose,
}: {
  role?: RbacRole;
  roles: RbacRole[];
  definitions: RbacDefinition[];
  grantAll: boolean;
  onSubmit: (input: RbacRoleInput) => Promise<void>;
  onClose: () => void;
}) {
  const [name, setName] = useState(role?.name ?? "");
  const [description, setDescription] = useState(role?.description ?? "");
  const [rules, setRules] = useState<RbacRoleRule[]>(role?.rules ?? []);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [openBranchId, setOpenBranchId] = useState<string | null>(null);

  const branches = permissionBranches.buildBranches(definitions, rules, role?.rules ?? [], grantAll);
  const openBranch = branches.find((branch) => branch.id === openBranchId);

  const submit = async () => {
    const result = permissionForms.validateRole({ name, description, rules }, roles, definitions, role?.id, grantAll);
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
      wide
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
          <div class="text-xs uppercase tracking-[0.08em] text-ink-300">Permissions</div>
          <div class="text-[11.5px] text-ink-300">
            {rules.length} rule{rules.length === 1 ? "" : "s"} selected
          </div>
        </div>
        {openBranch ? (
          <BranchConfigurator
            key={openBranch.id}
            branch={openBranch}
            disabled={busy}
            onBack={() => setOpenBranchId(null)}
            onSetEntry={(key, state) => setRules((current) => permissionBranches.withState(current, key, state))}
            onSetBranch={(state) =>
              setRules((current) => permissionBranches.withBranchState(current, openBranch, state))
            }
          />
        ) : branches.length === 0 ? (
          <div class="text-[12.5px] text-ink-300">No permissions are available to grant.</div>
        ) : (
          <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
            {branches.map((branch) => (
              <BranchOverviewCard key={branch.id} branch={branch} onOpen={() => setOpenBranchId(branch.id)} />
            ))}
          </div>
        )}
      </div>

      {err && <div class="text-xs text-accent-red">{err}</div>}
    </PermissionModalShell>
  );
}
