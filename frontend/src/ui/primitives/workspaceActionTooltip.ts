import { shortcutService } from "../../services/platform/shortcutService.ts";

export function workspaceActionTooltipClass(placement: "below" | "left", open: boolean): string {
  const base = "workspace-action-tooltip pointer-events-none absolute z-50 whitespace-nowrap rounded-control border border-line bg-raised px-2 py-1 text-[11px] font-medium text-ink-100 shadow-pop transition-[opacity,transform] duration-150 motion-reduce:transition-none";
  return `${base} ${placement === "below"
    ? `right-0 top-full mt-2 ${open ? "translate-y-0 opacity-100" : "-translate-y-1 opacity-0"}`
    : `right-full top-1/2 mr-2 -translate-y-1/2 ${open ? "translate-x-0 opacity-100" : "translate-x-1 opacity-0"}`}`;
}

let nextTooltipId = 0;

/** Give DOM-rendered extension actions the workspace toolbar's tooltip. */
export function attachWorkspaceActionTooltip(
  button: HTMLButtonElement,
  text: string,
  placement: "below" | "left" = "below",
): () => void {
  const tooltip = document.createElement("span");
  tooltip.id = `workspace-extension-tooltip-${++nextTooltipId}`;
  tooltip.setAttribute("role", "tooltip");
  tooltip.textContent = text;
  button.classList.add("relative");
  button.setAttribute("aria-describedby", tooltip.id);
  button.appendChild(tooltip);

  let hovered = false;
  let focused = false;
  let dismissed = false;
  const isOpen = () => !dismissed && (hovered || focused);
  const update = () => {
    tooltip.className = workspaceActionTooltipClass(placement, isOpen());
    tooltip.setAttribute("aria-hidden", String(!isOpen()));
  };
  const events = new AbortController();
  const options = { signal: events.signal };
  button.addEventListener("mouseenter", () => {
    hovered = true;
    dismissed = false;
    update();
  }, options);
  button.addEventListener("mouseleave", () => {
    hovered = false;
    update();
  }, options);
  button.addEventListener("focus", () => {
    focused = true;
    dismissed = false;
    update();
  }, options);
  button.addEventListener("blur", () => {
    focused = false;
    dismissed = false;
    update();
  }, options);
  button.addEventListener("keydown", (event) => {
    if (!isOpen() || !shortcutService.isDismiss(event)) return;
    dismissed = true;
    event.stopPropagation();
    update();
  }, options);
  update();

  return () => {
    events.abort();
    button.removeAttribute("aria-describedby");
    tooltip.remove();
  };
}
