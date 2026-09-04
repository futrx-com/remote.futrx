import type { JSX } from "preact";

export function Loading({ text }: { text: string }) {
  return (
    <div class="rounded-md border border-line bg-tint px-3 py-4 text-center text-[12.5px] text-ink-300">
      {text}
    </div>
  );
}

export function Empty({ text, compact }: { text: string; compact?: boolean }) {
  return (
    <div
      class={`rounded-card border border-line bg-surface ${
        compact ? "px-3 py-2.5" : "px-4 py-5"
      } text-sm text-ink-300`}
    >
      {text}
    </div>
  );
}

export function Panel({ title, children }: { title: string; children: JSX.Element | JSX.Element[] }) {
  return (
    <section class="rounded-md border border-line bg-tint overflow-hidden">
      <header class="px-3 py-2 border-b border-line">
        <h3 class="text-[11.5px] font-semibold uppercase tracking-wide text-ink-300">{title}</h3>
      </header>
      <div class="p-2.5">{children}</div>
    </section>
  );
}

export function Grid({ children }: { children: JSX.Element | JSX.Element[] }) {
  return <div class="grid gap-2 sm:grid-cols-2">{children}</div>;
}

export function Field({
  label,
  value,
  mono,
  tone,
}: {
  label: string;
  value: string;
  mono?: boolean;
  tone?: "warn";
}) {
  return (
    <div class="rounded-md border border-line bg-tint px-3 py-2 min-w-0">
      <div class="text-[11px] text-ink-400">{label}</div>
      <div
        class={`mt-0.5 text-[12.5px] truncate ${mono ? "font-mono" : ""} ${
          tone === "warn" ? "text-accent-orange" : "text-ink-100"
        }`}
        title={value}
      >
        {value}
      </div>
    </div>
  );
}
