import { useEffect, useState } from "preact/hooks";
import { requestJson } from "../../api/apiRequest";

type Instructions = { global: string; providers: Record<string, string> };
type Settings = { instructions: Instructions; targets: Record<string, string> };
const endpoint = "/api/admin/agent-instructions";

export function AgentInstructionsSettings() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    let active = true;
    requestJson<Settings>("GET", endpoint).then(value => {
      if (active) setSettings({ ...value, instructions: { global: value.instructions.global || "", providers: value.instructions.providers || {} } });
    }).catch(err => { if (active) setError(String(err)); });
    return () => { active = false; };
  }, []);
  async function save() {
    if (!settings) return;
    setSaving(true); setError(""); setSaved(false);
    try { await requestJson("PUT", endpoint, settings.instructions); setSaved(true); }
    catch (err) { setError(String(err)); }
    finally { setSaving(false); }
  }
  return <section class="space-y-3 border-t border-line pt-4">
    <h3 class="font-semibold">Global agent instructions</h3>
    <p class="text-sm text-ink-300">Applied to every project alongside Remote’s built-in instructions. Project instructions can supplement these. Restart the backend after saving; subsequent project provisioning publishes the updated files.</p>
    {error && <p role="alert">{error}</p>}
    {!settings && !error && <p>Loading instructions…</p>}
    {settings && <>
      <label class="block">All providers<textarea aria-label="Global instructions" class="block w-full rounded border border-line bg-surface p-2" rows={8} disabled={saving} value={settings.instructions.global} onInput={e => { setSaved(false); setSettings({ ...settings, instructions: { ...settings.instructions, global: e.currentTarget.value } }); }} /></label>
      {Object.entries(settings.targets).sort().map(([provider, target]) => <label class="block" key={provider}>{provider} — {target}<textarea aria-label={`${provider} instructions`} class="block w-full rounded border border-line bg-surface p-2" rows={5} disabled={saving} value={settings.instructions.providers[provider] || ""} onInput={e => { setSaved(false); setSettings({ ...settings, instructions: { ...settings.instructions, providers: { ...settings.instructions.providers, [provider]: e.currentTarget.value } } }); }} /></label>)}
      <button type="button" class="rounded border border-line px-3 py-2" disabled={saving} onClick={() => void save()}>{saving ? "Saving…" : "Save instructions"}</button>
      {saved && <p role="status">Saved. Restart the backend to apply these instructions.</p>}
    </>}
  </section>;
}
