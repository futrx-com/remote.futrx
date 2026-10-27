import { useLayoutEffect, useRef } from "preact/hooks";

export function useRevealHeight<T extends HTMLElement>(animate: boolean, contentVersion: number) {
  const elementRef = useRef<T>(null);
  const initialized = useRef(false);
  const targetHeight = useRef(0);
  const animating = useRef(false);

  useLayoutEffect(() => {
    const element = elementRef.current;
    if (!animate || !element) return;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      targetHeight.current = element.getBoundingClientRect().height;
      initialized.current = true;
      animating.current = false;
      element.style.height = "";
      element.style.overflow = "";
      element.style.maskImage = "";
      element.style.removeProperty("--stream-reveal-feather");
      return;
    }

    // Lists can gain items after mounting. Start from the visible height, then
    // measure the new natural height so one clean edge opens below the text.
    const from = !initialized.current ? 0
      : animating.current ? element.getBoundingClientRect().height : targetHeight.current;
    initialized.current = true;
    element.style.height = "auto";
    const to = element.getBoundingClientRect().height;
    targetHeight.current = to;
    if (to <= from + 0.5) {
      animating.current = false;
      element.style.height = "";
      element.style.overflow = "";
      element.style.maskImage = "";
      element.style.removeProperty("--stream-reveal-feather");
      return;
    }
    // Match duration to the remaining distance. With a linear transition,
    // repeated item arrivals keep roughly the same visual speed instead of
    // restarting a slow ease at every row.
    const duration = Math.min(1200, Math.max(80, ((to - from) / 900) * 1000));
    element.style.setProperty("--stream-reveal-duration", `${duration}ms`);
    const softenEdge = element.matches("ul, ol") || element.querySelector(":scope > table") !== null
      || to - from > 80 || !!element.style.maskImage;
    if (softenEdge) {
      element.style.maskImage = "linear-gradient(to bottom, black calc(100% - var(--stream-reveal-feather)), transparent 100%)";
      element.style.setProperty("--stream-reveal-feather", "18px");
    }
    animating.current = true;
    element.style.height = `${from}px`;
    element.style.overflow = "hidden";
    void element.offsetHeight;
    const frame = requestAnimationFrame(() => {
      element.style.height = `${to}px`;
      if (softenEdge) element.style.setProperty("--stream-reveal-feather", "0px");
    });
    const finish = (event: TransitionEvent) => {
      if (event.target !== element || event.propertyName !== "height") return;
      element.style.height = "";
      element.style.overflow = "";
      element.style.maskImage = "";
      element.style.removeProperty("--stream-reveal-feather");
      animating.current = false;
      element.removeEventListener("transitionend", finish);
    };
    element.addEventListener("transitionend", finish);
    return () => {
      cancelAnimationFrame(frame);
      element.removeEventListener("transitionend", finish);
    };
  }, [animate, contentVersion]);
  return elementRef;
}
