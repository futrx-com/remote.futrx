import assert from "node:assert/strict";
import test, { type TestContext } from "node:test";

import { applicationsApi } from "../../api/applicationsApi.ts";
import { EXTENSION_SLOTS } from "../../config/extensions.ts";
import type { AppUIExtension } from "../../models/application.ts";
import type { ExtensionApi, ExtensionRegistry } from "../../models/extension.ts";
import { extensionEventService } from "../../services/extensions/extensionEventService.ts";
import { createExtensionStore } from "../../state/stores/extensions/extensionStore.ts";
import { ExtensionHost, type LoadEntryModule } from "./extensionHost.ts";

type Listener = () => void;

/**
 * The host reads `document` the way production code does, so the listener it
 * registers is observable only through a stand-in installed for the case.
 */
function installDocument(t: TestContext) {
  let visibility: DocumentVisibilityState = "visible";
  const listeners = new Map<string, Listener[]>();
  const invoke = (listener: EventListenerOrEventListenerObject, type: string) =>
    typeof listener === "function"
      ? () => listener(new Event(type))
      : () => listener.handleEvent(new Event(type));
  const handlers = new Map<EventListenerOrEventListenerObject, Listener>();

  const fakeDocument = {
    get visibilityState() {
      return visibility;
    },
    addEventListener: (type: string, listener: EventListenerOrEventListenerObject) => {
      const handler = invoke(listener, type);
      handlers.set(listener, handler);
      listeners.set(type, [...(listeners.get(type) ?? []), handler]);
    },
    removeEventListener: (
      type: string,
      listener: EventListenerOrEventListenerObject,
    ) => {
      const handler = handlers.get(listener);
      listeners.set(
        type,
        (listeners.get(type) ?? []).filter((candidate) => candidate !== handler),
      );
    },
  } as unknown as Document;

  const previous = Object.getOwnPropertyDescriptor(globalThis, "document");
  Object.defineProperty(globalThis, "document", {
    configurable: true,
    writable: true,
    value: fakeDocument,
  });
  t.after(() => {
    if (previous) Object.defineProperty(globalThis, "document", previous);
    else Reflect.deleteProperty(globalThis, "document");
  });

  return {
    listenerCount: (type: string) => listeners.get(type)?.length ?? 0,
    setVisibility: (value: DocumentVisibilityState) => {
      visibility = value;
    },
    dispatch: (type: string) => {
      for (const listener of [...(listeners.get(type) ?? [])]) listener();
    },
  };
}

/** Answers `GET /api/applications/ui` from a script of successive replies. */
function serveExtensions(
  t: TestContext,
  replies: (AppUIExtension[] | Error)[],
): { calls: number } {
  const original = applicationsApi.uiExtensions;
  const state = { calls: 0 };
  applicationsApi.uiExtensions = async () => {
    const reply = replies[Math.min(state.calls, replies.length - 1)];
    state.calls += 1;
    if (reply instanceof Error) throw reply;
    return reply ?? [];
  };
  t.after(() => {
    applicationsApi.uiExtensions = original;
  });
  return state;
}

function createRegistry() {
  const store = createExtensionStore();
  const registry: ExtensionRegistry = {
    register: (...args) => store.getState().register(...args),
    setVisibility: (...args) => store.getState().setVisibility(...args),
    removeImage: (...args) => store.getState().removeImage(...args),
  };
  return {
    registry,
    panelContributions: () =>
      store.getState().bySlot.get(EXTENSION_SLOTS.applicationsPanel) ?? [],
  };
}

function helloExtension(global = true): AppUIExtension {
  return {
    image: {
      id: "hello-remote",
      name: "Hello Remote",
      type: "backend",
      scopes: ["global"],
      port: { internal: 0, defaultExternal: 0, protocol: "" },
      ui: { entry: "scripts/main.js" },
    },
    global,
    projectIds: [],
  };
}

/**
 * Stands in for an image's `ui/scripts/main.js`: it contributes one panel and
 * one event handler, which is what "the extension is loaded" means from the
 * outside.
 */
function entryModuleLoader(): {
  load: LoadEntryModule;
  urls: string[];
  events: string[];
} {
  const urls: string[] = [];
  const events: string[] = [];
  const load: LoadEntryModule = async (url) => {
    urls.push(url);
    return {
      default: (remote: ExtensionApi) => {
        remote.ui.register(remote.slots.applicationsPanel, () => {});
        remote.events.on("upload.completed", () => {
          events.push(remote.image.id);
        });
      },
    };
  };
  return { load, urls, events };
}

/**
 * Lets every pending microtask settle. A sync is fetch -> reconcile -> import
 * -> activate with no timers in between, so one turn of the event loop is the
 * whole of it, and awaiting `sync()` again would start a second one.
 */
function settled(): Promise<void> {
  return new Promise((resolve) => setImmediate(resolve));
}

function emitUploadCompleted(): void {
  extensionEventService.emit("upload.completed", {
    chatId: "chat-1",
    fileName: "notes.txt",
    directory: "/workspace/.uploads",
    path: "/workspace/.uploads/notes.txt",
    size: 1,
    claim: () => {},
  });
}

test("watching syncs immediately and loads the installed extension", async (t) => {
  const browser = installDocument(t);
  const served = serveExtensions(t, [[helloExtension()]]);
  const { registry, panelContributions } = createRegistry();
  const entry = entryModuleLoader();
  const host = new ExtensionHost(registry, entry.load);

  const stopWatching = host.watch();
  await settled();

  assert.equal(served.calls, 1);
  assert.deepEqual(entry.urls, [
    "/api/applications/catalog/hello-remote/ui/scripts/main.js",
  ]);
  assert.equal(panelContributions().length, 1);
  assert.equal(browser.listenerCount("visibilitychange"), 1);

  stopWatching();
});

test("returning to the foreground drops an extension uninstalled elsewhere", async (t) => {
  const browser = installDocument(t);
  serveExtensions(t, [[helloExtension()], []]);
  const { registry, panelContributions } = createRegistry();
  const entry = entryModuleLoader();
  const host = new ExtensionHost(registry, entry.load);

  const stopWatching = host.watch();
  await settled();
  assert.equal(panelContributions().length, 1);

  browser.dispatch("visibilitychange");
  await settled();

  assert.equal(panelContributions().length, 0);
  emitUploadCompleted();
  assert.deepEqual(entry.events, [], "its event handlers go with it");

  stopWatching();
});

test("a hidden tab does not sync", async (t) => {
  const browser = installDocument(t);
  const served = serveExtensions(t, [[helloExtension()]]);
  const { registry } = createRegistry();
  const host = new ExtensionHost(registry, entryModuleLoader().load);

  const stopWatching = host.watch();
  await settled();

  browser.setVisibility("hidden");
  browser.dispatch("visibilitychange");
  await settled();

  assert.equal(served.calls, 1, "only the sync that started the watch ran");

  stopWatching();
});

test("disposing stops watching", async (t) => {
  const browser = installDocument(t);
  const served = serveExtensions(t, [[helloExtension()]]);
  const { registry } = createRegistry();
  const host = new ExtensionHost(registry, entryModuleLoader().load);

  const stopWatching = host.watch();
  await settled();

  stopWatching();
  browser.dispatch("visibilitychange");
  await settled();

  assert.equal(browser.listenerCount("visibilitychange"), 0);
  assert.equal(served.calls, 1);
});

test("re-syncing an unchanged catalog neither re-imports nor re-registers", async (t) => {
  const browser = installDocument(t);
  serveExtensions(t, [[helloExtension()]]);
  const { registry, panelContributions } = createRegistry();
  const entry = entryModuleLoader();
  const host = new ExtensionHost(registry, entry.load);

  const stopWatching = host.watch();
  await settled();
  browser.dispatch("visibilitychange");
  await settled();
  browser.dispatch("visibilitychange");
  await settled();

  assert.equal(entry.urls.length, 1);
  assert.equal(panelContributions().length, 1);

  stopWatching();
});

test("a failed extension list leaves what is loaded alone", async (t) => {
  installDocument(t);
  serveExtensions(t, [[helloExtension()], new Error("offline")]);
  const { registry, panelContributions } = createRegistry();
  const host = new ExtensionHost(registry, entryModuleLoader().load);

  await host.sync();
  await host.sync();

  assert.equal(panelContributions().length, 1);
});
