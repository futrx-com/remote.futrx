import { AlertCircle } from "../../primitives/icons";

export function ErrorMessage({ message }: { message: string }) {
  return (
    <div class="flex items-start gap-2 text-accent-red text-sm rounded-card border border-accent-red/25 bg-accent-red/[0.08] px-3 py-2">
      <AlertCircle class="w-4 h-4 flex-none mt-0.5" />
      <div class="min-w-0 flex-1 [overflow-wrap:anywhere]">{message}</div>
    </div>
  );
}
