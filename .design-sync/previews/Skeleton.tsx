// Skeleton — the placeholder bar every loading surface is built from.
import { Skeleton } from "remote.futrx-web";

export const Sizes = () => (
  <div className="w-full max-w-sm space-y-3">
    <Skeleton class="h-2.5 w-full" />
    <Skeleton class="h-2.5 w-[70%]" />
    <Skeleton class="h-2.5 w-[45%]" />
  </div>
);

export const Shapes = () => (
  <div className="flex w-full max-w-sm items-center gap-3">
    <Skeleton class="h-9 w-9 flex-none rounded-full" />
    <Skeleton class="h-8 w-8 flex-none rounded-control" />
    <Skeleton class="h-9 flex-1 rounded-card" />
  </div>
);
