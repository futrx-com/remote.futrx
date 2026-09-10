export interface ApprovalSummaryField {
  label: string;
  value: string;
}

const FIELD_SOURCES: Array<{ label: string; keys: string[] }> = [
  { label: "Command", keys: ["command", "cmd"] },
  { label: "Action", keys: ["action", "operation", "request"] },
  { label: "Reason", keys: ["reason", "why", "description"] },
];

export function approvalSummaryFields(input: Record<string, unknown>): ApprovalSummaryField[] {
  return FIELD_SOURCES.flatMap(({ label, keys }) => {
    const value = firstDisplayValue(input, keys);
    return value ? [{ label, value }] : [];
  });
}

function firstDisplayValue(input: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = input[key];
    const rendered = renderValue(value);
    if (rendered) return rendered;
  }
  return "";
}

function renderValue(value: unknown): string {
  if (typeof value === "string") return value.trim();
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (value === null || value === undefined) return "";
  try {
    return JSON.stringify(value);
  } catch {
    return "";
  }
}
