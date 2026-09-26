import type { AppApplication, AppInstallRequest } from "../../models/application";

export function prepareInstallRequest(
  application: AppApplication,
  input: { name: string; env: Record<string, string>; externalPort: string },
  asksForPort: boolean,
): AppInstallRequest {
  const port = asksForPort && input.externalPort.trim()
    ? Number(input.externalPort.trim())
    : undefined;
  if (port !== undefined && (!Number.isInteger(port) || port < 1 || port > 65535)) {
    throw new Error("External port must be 1–65535.");
  }
  for (const variable of application.env ?? []) {
    if (variable.format !== "json") continue;
    const value = input.env[variable.key] ?? variable.default ?? "";
    if (!value.trim()) continue;
    if (new TextEncoder().encode(value).length > 128 * 1024) {
      throw new Error(`${variable.label || variable.key} must be smaller than 128 KiB.`);
    }
    let parsed: unknown;
    try {
      parsed = JSON.parse(value);
    } catch {
      throw new Error(`${variable.label || variable.key} must be valid JSON.`);
    }
    if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new Error(`${variable.label || variable.key} must be a JSON object.`);
    }
  }
  return {
    applicationId: application.id,
    name: input.name.trim() || application.name,
    env: input.env,
    externalPort: port,
  };
}
