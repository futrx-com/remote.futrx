import { useState } from "preact/hooks";
import type { ProjectMeta } from "../../../models/project";
import type { RbacBindingInput, RbacDefinition, RbacRole, RbacScopeKind } from "../../../models/rbac";
import { permissionForms } from "../../../state/hooks/permissions/permissionForms";
import { INPUT_CLASS, SELECT_CLASS, ScopeFields } from "./PermissionFormFields";

export function AddBindingForm({
  roles,
  definitions,
  projects,
  onAdd,
}: {
  roles: RbacRole[];
  definitions: RbacDefinition[];
  projects: ProjectMeta[];
  onAdd: (input: RbacBindingInput) => Promise<void>;
}) {
  const [userEmail, setUserEmail] = useState("");
  const [roleId, setRoleId] = useState("");
  const [scopeKind, setScopeKind] = useState<RbacScopeKind>("platform");
  const [projectId, setProjectId] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const offered = permissionForms.scopeKindsForRole(
    definitions,
    roles.find((role) => role.id === roleId)
  );
  const kind = permissionForms.effectiveScopeKind(offered, scopeKind);

  const submit = async (e: Event) => {
    e.preventDefault();
    const result = permissionForms.validateBinding(
      { userEmail, roleId, scopeKind: kind, projectId },
      roles,
      definitions
    );
    if (!result.ok) {
      setErr(result.message);
      return;
    }
    setErr(null);
    setSubmitting(true);
    try {
      await onAdd(result.input);
      setUserEmail("");
      setProjectId("");
    } catch (cause) {
      setErr((cause as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} class="rounded-md border border-line bg-tint p-2.5 space-y-2">
      <div class="grid gap-2 sm:grid-cols-2 items-center">
        <input
          type="email"
          value={userEmail}
          onInput={(e) => setUserEmail((e.target as HTMLInputElement).value)}
          placeholder="someone@example.com"
          spellcheck={false}
          autoComplete="off"
          class={INPUT_CLASS}
        />
        <select
          value={roleId}
          onChange={(e) => setRoleId((e.target as HTMLSelectElement).value)}
          class={SELECT_CLASS}
          aria-label="Role"
        >
          <option value="">Choose role…</option>
          {roles.map((role) => (
            <option key={role.id} value={role.id}>{role.name}</option>
          ))}
        </select>
        <ScopeFields
          offered={offered}
          kind={kind}
          projectId={projectId}
          projects={projects}
          onKindChange={setScopeKind}
          onProjectChange={setProjectId}
        />
        <button type="submit" disabled={submitting} class="btn btn-primary btn-sm disabled:opacity-50">
          {submitting ? "Binding…" : "Bind role"}
        </button>
      </div>
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </form>
  );
}
