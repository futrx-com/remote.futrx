import { Loader } from "./icons";

export function LoadingScreen() {
  return (
    <div class="app-shell grid place-items-center bg-app text-ink-300">
      <div class="grid place-items-center gap-3">
        <Loader class="w-6 h-6 animate-spin" />
        <span class="text-sm">Loading</span>
      </div>
    </div>
  );
}
