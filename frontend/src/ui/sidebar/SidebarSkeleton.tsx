import { Skeleton } from "../primitives/Skeleton";

// Fixed widths, not random ones: a placeholder that reshuffles between renders
// reads as content changing rather than content arriving. The shape traces
// ProjectGroup and ChatRow — chevron, name, then indented rows carrying an
// icon, a title and an age — so the real list lands on top of the placeholder
// instead of replacing a different layout.
const PLACEHOLDER_GROUPS = [
  { header: "w-[42%]", chats: ["w-[70%]", "w-[54%]", "w-[63%]", "w-[46%]", "w-[58%]"] },
  { header: "w-[30%]", chats: ["w-[61%]", "w-[74%]", "w-[49%]", "w-[66%]"] },
  { header: "w-[48%]", chats: ["w-[57%]", "w-[68%]", "w-[44%]"] },
];

// Ages are short and near-uniform in the real list, so they vary only slightly.
const AGE_WIDTHS = ["w-5", "w-6", "w-4"];

export function SidebarSkeleton() {
  return (
    <div role="status" aria-label="Loading projects" class="space-y-2.5 pt-1">
      {PLACEHOLDER_GROUPS.map((group, groupIndex) => (
        <div key={groupIndex}>
          <div class="flex items-center gap-1.5 py-1.5 pl-1 pr-1">
            <Skeleton class="h-3.5 w-3.5 flex-none rounded" />
            <Skeleton class={`h-2 ${group.header}`} />
          </div>
          <div class="mt-1.5 space-y-px pl-3 pr-0.5">
            {group.chats.map((width, chatIndex) => (
              <div key={chatIndex} class="flex h-8 items-center gap-2 rounded-control pl-2 pr-2.5">
                <Skeleton class="h-3.5 w-3.5 flex-none rounded" />
                <Skeleton class={`h-2.5 ${width}`} />
                <div class="flex-1" />
                <Skeleton class={`h-2 flex-none ${AGE_WIDTHS[chatIndex % AGE_WIDTHS.length]}`} />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
