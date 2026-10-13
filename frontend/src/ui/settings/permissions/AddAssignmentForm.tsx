import { useState } from "preact/hooks";
import type { ProjectMeta } from "../../../models/project";
import type { RbacAssignmentInput, RbacDefinition, RbacEffect, RbacScopeKind } from "../../../models/rbac";
import { permissionForms } from "../../../state/hooks/permissions/permissionForms";
import { INPUT_CLASS, SELECT_CLASS, ScopeFields } from "./PermissionFormFields";

export function AddAssignmentForm({
  definitions,
  projects,
  onAdd,
}: {
  definitions: RbacDefinition[];
  projects: ProjectMeta[];
  onAdd: (input: RbacAssignmentInput) => Promise<void>;
}) {
  const [userEmail, setUserEmail] = useState("");
  const [permission, setPermission] = useState("");
  const [effect, setEffect] = useState<RbacEffect>("allow");
  const [scopeKind, setScopeKind] = useState<RbacScopeKind>("platform");
  const [projectId, setProjectId] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const offered = permissionForms.scopeKindsForPermission(definitions, permission);
  const kind = permissionForms.effectiveScopeKind(offered, scopeKind);

  const submit = async (e: Event) => {
    e.preventDefault();
    const result = permissionForms.validateAssignment(
      { userEmail, permission, effect, scopeKind: kind, projectId },
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
          value={permission}
          onChange={(e) => setPermission((e.target as HTMLSelectElement).value)}
          class={`${SELECT_CLASS} font-mono`}
          aria-label="Permission"
        >
          <option value="">Choose permission…</option>
          {definitions.map((definition) => (
            <option key={definition.key} value={definition.key}>{definition.key}</option>
          ))}
        </select>
        <select
          value={effect}
          onChange={(e) => setEffect((e.target as HTMLSelectElement).value as RbacEffect)}
          class={SELECT_CLASS}
          aria-label="Effect"
        >
          <option value="allow">allow</option>
          <option value="deny">deny</option>
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
          {submitting ? "Saving…" : "Set assignment"}
        </button>
      </div>
      {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
    </form>
  );
}
