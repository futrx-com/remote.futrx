export function formatUpdateRelativeTime(unixSeconds: number, now = Date.now()): string {
  const elapsedSeconds = Math.max(0, Math.round(now / 1000 - unixSeconds));
  if (elapsedSeconds < 5) return "just now";
  if (elapsedSeconds < 60) return `${elapsedSeconds} seconds ago`;
  const elapsedMinutes = Math.floor(elapsedSeconds / 60);
  return `${elapsedMinutes} minute${elapsedMinutes === 1 ? "" : "s"} ago`;
}

export function formatUpdateTime(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
