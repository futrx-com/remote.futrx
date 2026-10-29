import { TOKENS, COMPACT } from "../config/drawer.js";

export function syncDrawerTheme(frame, send) {
  const compact = window.matchMedia(COMPACT);
  const theme = () => {
    const styles = getComputedStyle(document.documentElement);
    send({
      type: "theme",
      tokens: Object.fromEntries(
        TOKENS.map((name) => [name, styles.getPropertyValue(name).trim()]),
      ),
      compact: compact.matches,
    });
  };
  frame.addEventListener("load", theme);
  compact.addEventListener("change", theme);
  const observer = new MutationObserver(theme);
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["class", "data-theme", "style"],
  });
  return () => {
    observer.disconnect();
    compact.removeEventListener("change", theme);
    frame.removeEventListener("load", theme);
  };
}
