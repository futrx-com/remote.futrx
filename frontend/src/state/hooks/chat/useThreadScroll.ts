import { useEffect, useRef, useState } from "preact/hooks";

export function useThreadScroll(resetKey: string, scrollKey: unknown) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const userScrolledRef = useRef(false);
  // Set by a jump to an older message. Programmatic scrolls can pass near the
  // bottom while history loads, so only the user's own scrolling releases it.
  const heldRef = useRef(false);
  const [showJump, setShowJump] = useState(false);

  useEffect(() => {
    unlockAutoScroll();
    setShowJump(false);
  }, [resetKey]);

  useEffect(() => {
    // ResizeObserver follows the visible content as a block opens. Stream
    // events can arrive without changing its height; scheduling a second
    // scroll for every event makes the viewport chase stale positions.
    if (userScrolledRef.current || typeof ResizeObserver !== "undefined") return;
    scrollToBottom("auto");
  }, [scrollKey]);

  useEffect(() => {
    const content = contentRef.current;
    if (!content || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => {
      const element = scrollRef.current;
      if (!userScrolledRef.current && element) element.scrollTop = element.scrollHeight;
    });
    observer.observe(content);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const element = scrollRef.current;
    if (!element) return;
    const release = () => { heldRef.current = false; };
    element.addEventListener("wheel", release, { passive: true });
    element.addEventListener("touchstart", release, { passive: true });
    element.addEventListener("keydown", release);
    return () => {
      element.removeEventListener("wheel", release);
      element.removeEventListener("touchstart", release);
      element.removeEventListener("keydown", release);
    };
  }, []);

  function onScroll() {
    const element = scrollRef.current;
    if (!element) return;
    const nearBottom = !heldRef.current &&
      element.scrollHeight - element.scrollTop - element.clientHeight < 96;
    userScrolledRef.current = !nearBottom;
    setShowJump((current) => (current === !nearBottom ? current : !nearBottom));
  }

  function scrollToBottom(behavior: ScrollBehavior) {
    requestAnimationFrame(() => {
      bottomRef.current?.scrollIntoView({ block: "end", behavior });
    });
  }

  function unlockAutoScroll() {
    heldRef.current = false;
    userScrolledRef.current = false;
  }

  /** Stop following the bottom, as a user scrolling up does, before a jump
   *  moves the thread to an older message. */
  function holdPosition() {
    heldRef.current = true;
    userScrolledRef.current = true;
    setShowJump(true);
  }

  function jumpToBottom() {
    unlockAutoScroll();
    setShowJump(false);
    scrollToBottom("smooth");
  }

  return {
    scrollRef,
    contentRef,
    bottomRef,
    showJump,
    onScroll,
    jumpToBottom,
    unlockAutoScroll,
    holdPosition,
  };
}
