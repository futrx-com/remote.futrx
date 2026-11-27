/**
 * Calls ready once element is shown and its size has held for a whole frame.
 * Remote animates a drawer open; rendering a tree into it while it grows
 * stalls that animation, so the first render waits for the pane to settle.
 */
export function whenPaneSettled(element, ready) {
  let frame = 0;
  let width = -1;
  let height = -1;
  let done = false;

  function check() {
    if (frame || done) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      if (done) return;
      const shown = element.clientWidth > 0 && element.clientHeight > 0;
      if (shown && element.clientWidth === width && element.clientHeight === height) {
        done = true;
        observer.disconnect();
        ready();
        return;
      }
      width = element.clientWidth;
      height = element.clientHeight;
      if (shown) check();
    });
  }

  const observer = new ResizeObserver(check);
  observer.observe(element);
  return () => {
    done = true;
    cancelAnimationFrame(frame);
    observer.disconnect();
  };
}
