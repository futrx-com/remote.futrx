export function isTerminalTurnStatus(status: string | undefined): boolean {
  return status === "completed" || status === "failed" || status === "interrupted";
}
