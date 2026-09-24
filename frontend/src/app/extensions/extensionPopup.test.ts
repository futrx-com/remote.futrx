import assert from "node:assert/strict";
import test from "node:test";

import { dismissStackService } from "../../services/platform/dismissStackService.ts";
import { shortcutService } from "../../services/platform/shortcutService.ts";
import { openExtensionPopup } from "./extensionPopup.ts";

class FakeEventTarget {
  readonly listeners = new Map<string, Set<EventListenerOrEventListenerObject>>();

  addEventListener(type: string, listener: EventListenerOrEventListenerObject): void {
    const listeners = this.listeners.get(type) ?? new Set();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  removeEventListener(type: string, listener: EventListenerOrEventListenerObject): void {
    this.listeners.get(type)?.delete(listener);
  }

  dispatch(type: string, event: Event): void {
    for (const listener of [...(this.listeners.get(type) ?? [])]) {
      if (typeof listener === "function") listener(event);
      else listener.handleEvent(event);
    }
  }
}

class FakeElement extends FakeEventTarget {
  readonly children: FakeElement[] = [];
  readonly attributes = new Map<string, string>();
  readonly style: Record<string, string> = {};
  parent: FakeElement | null = null;
  className = "";
  innerHTML = "";
  textContent: string | null = null;
  type = "";

  appendChild(child: FakeElement): FakeElement {
    child.parent = this;
    this.children.push(child);
    return child;
  }

  setAttribute(name: string, value: string): void {
    this.attributes.set(name, value);
  }

  remove(): void {
    if (!this.parent) return;
    const index = this.parent.children.indexOf(this);
    if (index >= 0) this.parent.children.splice(index, 1);
    this.parent = null;
  }
}

class FakeDocument extends FakeEventTarget {
  readonly body = new FakeElement();

  createElement(): FakeElement {
    return new FakeElement();
  }
}

const escapeEvent = {
  key: "Escape",
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  isComposing: false,
} as KeyboardEvent;

function installDOM(): {
  document: FakeDocument;
  window: FakeEventTarget;
  restore: () => void;
} {
  const originalDocument = globalThis.document;
  const originalWindow = globalThis.window;
  const document = new FakeDocument();
  const window = new FakeEventTarget();
  (globalThis as any).document = document;
  (globalThis as any).window = window;
  return {
    document,
    window,
    restore: () => {
      (globalThis as any).document = originalDocument;
      (globalThis as any).window = originalWindow;
    },
  };
}

test("media opened over an extension popup takes only the first Escape", () => {
  const dom = installDOM();
  let popupCleanups = 0;
  let mediaClaim = 0;
  let popup: ReturnType<typeof openExtensionPopup> | undefined;
  try {
    popup = openExtensionPopup({
      title: "Preview",
      mount: () => () => {
        popupCleanups += 1;
      },
    });

    mediaClaim = dismissStackService.claim();
    const onMediaKeyDown = (event: Event) => {
      const keyboardEvent = event as KeyboardEvent;
      if (!shortcutService.isDismiss(keyboardEvent)) return;
      if (!dismissStackService.owns(mediaClaim)) return;
      dismissStackService.release(mediaClaim);
    };
    dom.window.addEventListener("keydown", onMediaKeyDown);

    dom.window.dispatch("keydown", escapeEvent);

    assert.equal(popupCleanups, 0, "the popup remains behind the media viewer");
    assert.equal(dom.document.body.children.length, 1);

    dom.window.removeEventListener("keydown", onMediaKeyDown);
    dom.window.dispatch("keydown", escapeEvent);

    assert.equal(popupCleanups, 1, "the next Escape closes the revealed popup");
    assert.equal(dom.document.body.children.length, 0);
  } finally {
    dismissStackService.release(mediaClaim);
    popup?.close();
    dom.restore();
  }
});

test("a popup opened over another surface does not cascade dismissal", () => {
  const dom = installDOM();
  const behindClaim = dismissStackService.claim();
  let behindDismissals = 0;
  const onBehindKeyDown = (event: Event) => {
    const keyboardEvent = event as KeyboardEvent;
    if (!shortcutService.isDismiss(keyboardEvent)) return;
    if (!dismissStackService.owns(behindClaim)) return;
    behindDismissals += 1;
  };
  dom.window.addEventListener("keydown", onBehindKeyDown);
  let popup: ReturnType<typeof openExtensionPopup> | undefined;
  try {
    popup = openExtensionPopup();

    dom.window.dispatch("keydown", escapeEvent);

    assert.equal(dom.document.body.children.length, 0);
    assert.equal(behindDismissals, 0, "one key press closes only the popup");

    dom.window.dispatch("keydown", escapeEvent);
    assert.equal(behindDismissals, 1, "the revealed surface receives the next Escape");
  } finally {
    popup?.close();
    dom.window.removeEventListener("keydown", onBehindKeyDown);
    dismissStackService.release(behindClaim);
    dom.restore();
  }
});
