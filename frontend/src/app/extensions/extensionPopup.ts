import type {
  ExtensionPopupHandle,
  ExtensionPopupOptions,
} from "../../models/extension";

const OVERLAY_CLASS =
  "fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4";
const PANEL_CLASS =
  "flex max-h-[85vh] w-full flex-col overflow-hidden rounded-card border " +
  "border-line bg-surface text-ink-100 shadow-modal";

export function openExtensionPopup(
  options: ExtensionPopupOptions = {},
): ExtensionPopupHandle {
  const overlay = document.createElement("div");
  overlay.className = OVERLAY_CLASS;
  overlay.setAttribute("role", "dialog");
  overlay.setAttribute("aria-modal", "true");

  const panel = document.createElement("div");
  panel.className = PANEL_CLASS;
  panel.style.maxWidth = `${Math.max(280, options.width ?? 460)}px`;
  overlay.appendChild(panel);

  if (options.title) {
    const header = document.createElement("div");
    header.className =
      "flex items-center justify-between gap-3 border-b border-line px-4 py-3";
    const heading = document.createElement("h2");
    heading.className = "text-[13px] font-medium text-ink-50";
    heading.textContent = options.title;
    header.appendChild(heading);
    header.appendChild(closeButton(() => handle.close()));
    panel.appendChild(header);
  }

  const body = document.createElement("div");
  body.className = "overflow-y-auto px-4 py-3.5 text-[13px]";
  if (options.html) body.innerHTML = options.html;
  panel.appendChild(body);

  let unmount: (() => void) | void;
  let closed = false;
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === "Escape") handle.close();
  };
  const onOverlayClick = (event: MouseEvent) => {
    if (event.target === overlay) handle.close();
  };

  const handle: ExtensionPopupHandle = {
    body,
    close() {
      if (closed) return;
      closed = true;
      document.removeEventListener("keydown", onKeyDown);
      overlay.removeEventListener("click", onOverlayClick);
      try {
        if (typeof unmount === "function") unmount();
      } finally {
        overlay.remove();
      }
    },
  };

  overlay.addEventListener("click", onOverlayClick);
  document.addEventListener("keydown", onKeyDown);
  document.body.appendChild(overlay);
  unmount = options.mount?.(body);
  return handle;
}

function closeButton(onClick: () => void): HTMLButtonElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className =
    "grid h-7 w-7 flex-none place-items-center rounded-control text-ink-300 " +
    "transition hover:bg-tint-strong hover:text-ink-50";
  button.setAttribute("aria-label", "Close");
  button.innerHTML =
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
    'stroke-width="1.8" stroke-linecap="round" class="h-4 w-4">' +
    '<path d="M18 6 6 18M6 6l12 12"/></svg>';
  button.addEventListener("click", onClick);
  return button;
}
