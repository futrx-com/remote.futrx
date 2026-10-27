import { ICON } from "./config.js";

export function openPopover(anchor, onClose, options) {
  const panel = document.createElement("div");
  panel.className = "scheduled-tasks-popover";
  panel.setAttribute("role", "dialog");
  panel.setAttribute("aria-label", options.title);
  const header = document.createElement("header");
  const title = document.createElement("strong");
  title.textContent = options.title;
  const badge = document.createElement("span");
  badge.className = "scheduled-tasks-badge";
  badge.innerHTML = ICON;
  badge.setAttribute("aria-hidden", "true");
  const heading = document.createElement("div");
  const count = document.createElement("span");
  count.className = "scheduled-tasks-count";
  heading.append(title, count);
  const dismiss = document.createElement("button");
  dismiss.type = "button";
  dismiss.textContent = "×";
  dismiss.setAttribute("aria-label", "Close scheduled tasks");
  header.append(badge, heading, dismiss);
  const body = document.createElement("div");
  panel.append(header, body);
  document.body.append(panel);
  const cleanup = options.mount(body, { header, dismiss, count });
  let closed = false;
  const close = () => {
    if (closed) return;
    closed = true;
    cleanup?.();
    panel.remove();
    document.removeEventListener("keydown", key);
    onClose();
  };
  const key = (event) => { if (event.key === "Escape") { event.preventDefault(); close(); anchor.focus(); } };
  dismiss.addEventListener("click", () => { close(); anchor.focus(); });
  document.addEventListener("keydown", key);
  dismiss.focus();
  return { body, close };
}
