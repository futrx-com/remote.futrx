export function isPushPageFocused(): boolean {
  return document.visibilityState === "visible" && document.hasFocus();
}
