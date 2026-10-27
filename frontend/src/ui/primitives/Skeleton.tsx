// A single placeholder bar. Callers size it with utility classes so the
// placeholder can trace the real layout instead of a generic grey box.
export function Skeleton({ class: className = "" }: { class?: string }) {
  return <div aria-hidden="true" class={`skeleton ${className}`} />;
}
