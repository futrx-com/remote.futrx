/**
 * Human-readable summary of an approval interaction input.
 *
 * Provider payloads (e.g. Codex `item/commandExecution/requestApproval`) carry
 * machine fields (thread/exec IDs, timestamps) that mean nothing to most
 * users. Surface what the decision is about: why the agent asks (`reason`),
 * what it wants to run (`command`) and where (`cwd`). Anything else stays
 * available behind the raw "Request details" disclosure.
 */
export interface ApprovalSummary {
  reason?: string;
  command?: string;
  cwd?: string;
}

function nonEmptyString(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const trimmed = value.trim();
  return trimmed ? value : undefined;
}

export function summarizeApprovalRequest(input: Record<string, unknown>): ApprovalSummary {
  const summary: ApprovalSummary = {};
  const reason = nonEmptyString(input.reason);
  if (reason) summary.reason = reason;
  // A field literally named "command" is unambiguous: show it verbatim.
  const command = nonEmptyString(input.command);
  if (command) summary.command = command;
  const cwd = nonEmptyString(input.cwd);
  if (cwd) summary.cwd = cwd;
  return summary;
}
