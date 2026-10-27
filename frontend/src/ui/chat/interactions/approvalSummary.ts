export interface ApprovalSummaryField {
  label: string;
  value: string;
}

interface CommandActionSummary {
  command: string;
  action: string;
}

export function approvalSummaryFields(input: Record<string, unknown>): ApprovalSummaryField[] {
  const commandActions = summarizeCommandActions(input.commandActions);
  const fields: ApprovalSummaryField[] = [
    {
      label: "Command",
      value:
        joinedValues(commandActions, "command") || firstDisplayValue(input, ["command", "cmd"]),
    },
    {
      label: "Action",
      value:
        joinedValues(commandActions, "action") ||
        firstDisplayValue(input, ["action", "operation", "request"]),
    },
    { label: "Reason", value: firstDisplayValue(input, ["reason", "why", "description"]) },
    { label: "Directory", value: displayString(input.cwd) },
  ];

  return fields.flatMap(({ label, value }) => {
    return value ? [{ label, value }] : [];
  });
}

function summarizeCommandActions(value: unknown): CommandActionSummary[] {
  if (!Array.isArray(value)) return [];

  return value.flatMap((candidate) => {
    if (!isRecord(candidate)) return [];
    const command = displayString(candidate.command);
    if (!command) return [];
    const action = describeCommandAction(candidate);
    return [{ command, action }];
  });
}

function joinedValues(summaries: CommandActionSummary[], key: keyof CommandActionSummary): string {
  return summaries.map((summary) => summary[key]).filter(Boolean).join("\n");
}

function describeCommandAction(action: Record<string, unknown>): string {
  switch (displayString(action.type)) {
    case "read": {
      const target = firstDisplayString(action, ["name", "path"]);
      return target ? `Read ${target}` : "Read file";
    }
    case "listFiles": {
      const path = displayString(action.path);
      return path ? `List files in ${path}` : "List files";
    }
    case "search": {
      const query = displayString(action.query);
      const path = displayString(action.path);
      if (query && path) return `Search for ${query} in ${path}`;
      if (query) return `Search for ${query}`;
      return path ? `Search in ${path}` : "Search files";
    }
    case "unknown":
      return "Run command";
    default:
      return "";
  }
}

function firstDisplayString(input: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = displayString(input[key]);
    if (value) return value;
  }
  return "";
}

function displayString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
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
