import type { ComponentChildren } from "preact";
import type { RbacEffect, RbacScope } from "../../../models/rbac";
import { Loader } from "../../primitives/icons";

export function PermissionsSection({
  title,
  description,
  loading,
  children,
}: {
  title: string;
  description: string;
  loading?: boolean;
  children: ComponentChildren;
}) {
  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 flex items-start gap-3 border-b border-line">
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">{title}</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">{description}</div>
        </div>
        {loading && <Loader class="w-4 h-4 mt-2 text-ink-300 animate-spin" />}
      </header>
      <div class="p-3 space-y-3">{children}</div>
    </section>
  );
}

export function PermissionsRow({ children }: { children: ComponentChildren }) {
  return (
    <div class="rounded-md border border-line bg-tint px-3 py-2 space-y-1">
      <div class="flex items-center gap-2 min-w-0 flex-wrap">{children}</div>
    </div>
  );
}

export function PermissionBadge({
  tone,
  children,
}: {
  tone: "allow" | "deny" | "highlight" | "neutral";
  children: string;
}) {
  return (
    <span
      class={`inline-flex items-center h-5 px-1.5 rounded text-[11px] font-medium ${BADGE_TONES[tone]}`}
    >
      {children}
    </span>
  );
}

export function EffectBadge({ effect }: { effect: RbacEffect }) {
  return <PermissionBadge tone={effect}>{effect}</PermissionBadge>;
}

export function ScopeBadge({ scope }: { scope: RbacScope }) {
  return (
    <PermissionBadge tone="neutral">
      {scope.id ? `${scope.kind}:${scope.id}` : scope.kind}
    </PermissionBadge>
  );
}

const BADGE_TONES = {
  allow: "text-accent-green bg-accent-green/[0.10]",
  deny: "text-accent-red bg-accent-red/[0.10]",
  highlight: "text-accent-blue bg-accent-blue/[0.14]",
  neutral: "text-ink-300 bg-tint",
};
