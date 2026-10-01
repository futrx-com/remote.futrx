import { useEffect, useState } from "preact/hooks";
import { permissionsApi, type PermissionPolicy, type PermissionRule, type PermissionScope } from "../../api/permissionsApi";
import { projectApi } from "../../api/projectApi";
import type { ProjectMeta } from "../../models/project";

const field = "rounded-control border border-line bg-surface px-2 py-1.5 text-sm";
const scopeLabel = (scope: PermissionScope) => scope.kind === "platform" ? "Server" : `Project ${scope.id}`;

export function PermissionsSettings() {
  const [policy, setPolicy] = useState<PermissionPolicy | null>(null);
  const [projects, setProjects] = useState<ProjectMeta[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [email, setEmail] = useState("");
  const [projectId, setProjectId] = useState("");
  const [permission, setPermission] = useState("");
  const [effect, setEffect] = useState<"allow" | "deny">("deny");
  const [roleId, setRoleId] = useState("");
  const [editId, setEditId] = useState("");
  const [roleName, setRoleName] = useState("");
  const [roleDescription, setRoleDescription] = useState("");
  const [rules, setRules] = useState<PermissionRule[]>([]);
  const scope: PermissionScope = projectId ? { kind: "project", id: projectId } : { kind: "platform" };
  const refresh = async () => {
    const [next, visible] = await Promise.all([permissionsApi.policy(), projectApi.list()]);
    setPolicy(next); setProjects(visible);
  };
  useEffect(() => { void refresh().catch(e => setError(String(e.message || e))); }, []);
  const mutate = async (operation: () => Promise<unknown>) => {
    if (busy) return;
    setBusy(true); setError("");
    try { await operation(); await refresh(); window.dispatchEvent(new Event("permissions-changed")); }
    catch (e) { setError(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(false); }
  };
  const resetRole = () => { setEditId(""); setRoleName(""); setRoleDescription(""); setRules([]); };
  const eligible = policy?.definitions.filter(d => d.Scopes.includes(scope.kind)) || [];
  return <section class="rounded-card border border-line bg-surface p-4 space-y-4">
    <h2 class="font-semibold text-ink-50">Permissions</h2>
    <p class="text-sm text-ink-300">Deny overrides allow. Server rules apply only to server actions; project rules apply only to the selected project. Administrators always retain access.</p>
    {error && <p role="alert" class="text-accent-red">{error}</p>}
    {!policy ? <p class="text-sm text-ink-300">{error ? "Permission management is unavailable for this account." : "Loading permissions…"}</p> : <fieldset disabled={busy} class="space-y-5 disabled:opacity-60">
      <div class="flex flex-wrap gap-3">
        <label class="text-sm">User email<input type="email" class={`${field} block`} value={email} onInput={e => setEmail(e.currentTarget.value)} /></label>
        <label class="text-sm">Scope<select class={`${field} block`} value={projectId} onChange={e => { setProjectId(e.currentTarget.value); setPermission(""); setRoleId(""); }}>
          <option value="">Server</option>{projects.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select></label>
      </div>
      <form class="flex flex-wrap gap-2 items-end" onSubmit={e => { e.preventDefault(); void mutate(() => permissionsApi.assign({ UserEmail: email, Permission: permission, Effect: effect, Scope: scope })); }}>
        <label class="text-sm">Direct permission<select required class={`${field} block`} value={permission} onChange={e => setPermission(e.currentTarget.value)}>
          <option value="">Choose permission</option>{eligible.map(d => <option key={d.Key} value={d.Key}>{d.Description}</option>)}
        </select></label>
        <label class="text-sm">Effect<select class={`${field} block`} value={effect} onChange={e => setEffect(e.currentTarget.value as "allow" | "deny")}><option value="deny">Deny</option><option value="allow">Allow</option></select></label>
        <button class="btn btn-primary" disabled={!email || !permission}>Save permission</button>
      </form>
      <ul class="space-y-2 text-sm">{(policy.assignments || []).map(a => <li key={a.ID} class="flex flex-wrap gap-2 items-center">
        <span>{a.UserEmail} · {a.Effect} {a.Permission} · {scopeLabel(a.Scope)}</span>
        <button class="btn btn-secondary" onClick={() => void mutate(() => permissionsApi.removeAssignment(a))}>Remove rule</button>
      </li>)}</ul>
      <div class="border-t border-line pt-4 space-y-3">
        <h3 class="font-medium">{editId ? "Edit role" : "Create role"}</h3>
        <p class="text-sm text-ink-300">Roles group rules. Bind a role below to apply it to a user. Removing a rule restores the normal baseline unless another rule applies.</p>
        <form class="space-y-3" onSubmit={e => { e.preventDefault(); void mutate(async () => { await permissionsApi.saveRole({ Name: roleName, Description: roleDescription, Rules: rules }, editId || undefined); resetRole(); }); }}>
          <label class="text-sm block">Role name<input required maxLength={80} class={`${field} block w-full`} value={roleName} onInput={e => setRoleName(e.currentTarget.value)} /></label>
          <label class="text-sm block">Description<input maxLength={500} class={`${field} block w-full`} value={roleDescription} onInput={e => setRoleDescription(e.currentTarget.value)} /></label>
          {policy.definitions.map(d => <label key={d.Key} class="flex flex-wrap gap-2 justify-between text-sm">
            <span>{d.Description} <span class="text-ink-400">({d.Scopes.join(", ")})</span></span>
            <select class={field} value={rules.find(r => r.Permission === d.Key)?.Effect || ""} onChange={e => {
              const value = e.currentTarget.value; setRules(previous => [...previous.filter(r => r.Permission !== d.Key), ...(value ? [{ Permission: d.Key, Effect: value as "allow" | "deny" }] : [])]);
            }}><option value="">No rule</option><option value="deny">Deny</option><option value="allow">Allow</option></select>
          </label>)}
          <button class="btn btn-primary" disabled={!rules.length}>Save role</button>{editId && <button type="button" class="btn btn-secondary ml-2" onClick={resetRole}>Cancel edit</button>}
        </form>
        <ul class="space-y-2">{(policy.roles || []).map(role => <li key={role.ID} class="flex flex-wrap gap-2 items-center text-sm">
          <span>{role.Name} · {role.Rules.length} rules</span>
          <button class="btn btn-secondary" onClick={() => { setEditId(role.ID); setRoleName(role.Name); setRoleDescription(role.Description); setRules(role.Rules); }}>Edit</button>
          <button class="btn btn-secondary" disabled={(policy.bindings || []).some(b => b.RoleID === role.ID)} onClick={() => void mutate(() => permissionsApi.removeRole(role.ID))}>Delete unbound role</button>
        </li>)}</ul>
      </div>
      <form class="flex flex-wrap gap-2 items-end" onSubmit={e => { e.preventDefault(); void mutate(() => permissionsApi.bind({ UserEmail: email, RoleID: roleId, Scope: scope })); }}>
        <label class="text-sm">Role to bind<select required class={`${field} block`} value={roleId} onChange={e => setRoleId(e.currentTarget.value)}>
          <option value="">Choose role</option>{(policy.roles || []).filter(r => r.Rules.every(rule => eligible.some(d => d.Key === rule.Permission))).map(r => <option key={r.ID} value={r.ID}>{r.Name}</option>)}
        </select></label>
        <button class="btn btn-primary" disabled={!email || !roleId}>Bind to user and scope above</button>
      </form>
      <ul class="space-y-2 text-sm">{(policy.bindings || []).map(b => <li key={b.ID} class="flex flex-wrap gap-2 items-center">
        <span>{b.UserEmail} · {policy.roles?.find(r => r.ID === b.RoleID)?.Name || b.RoleID} · {scopeLabel(b.Scope)}</span>
        <button class="btn btn-secondary" onClick={() => void mutate(() => permissionsApi.unbind(b))}>Unbind</button>
      </li>)}</ul>
    </fieldset>}
  </section>;
}
