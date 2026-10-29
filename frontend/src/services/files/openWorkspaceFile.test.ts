import assert from "node:assert/strict";
import test from "node:test";
import { openWorkspaceFile, workspaceFileEditorUrl } from "./openWorkspaceFile.ts";
import { fileOpenerStore } from "../../state/stores/files/fileOpenerStore.ts";
import { mediaViewerStore } from "../../state/stores/media/mediaViewerStore.ts";

// Just enough of the browser for openWorkspaceFile: a page whose link
// handlers may claim a click, and tabs the browser may refuse to open.
function withBrowser(
  options: { claims?: (url: string) => boolean; blocksTabs?: boolean },
  run: (browser: { tabs: string[]; downloads: string[]; offered: string[] }) => void,
) {
  const browser = { tabs: [] as string[], downloads: [] as string[], offered: [] as string[] };
  const globals = ["window", "document", "MouseEvent"] as const;
  const previous = globals.map((name) => Object.getOwnPropertyDescriptor(globalThis, name));
  const define = (name: string, value: unknown) =>
    Object.defineProperty(globalThis, name, { configurable: true, writable: true, value });
  define("MouseEvent", Event);
  define("document", {
    body: { append: () => {} },
    createElement: () => {
      const listeners: ((event: Event) => void)[] = [];
      const link = {
        href: "",
        hidden: false,
        remove: () => {},
        addEventListener: (_type: string, listener: (event: Event) => void) => listeners.push(listener),
        dispatchEvent: (event: Event) => {
          browser.offered.push(link.href);
          if (options.claims?.(link.href)) return false;
          for (const listener of listeners) listener(event);
          return true;
        },
      };
      return link;
    },
  });
  define("window", {
    open: () => {
      if (options.blocksTabs) return null;
      const tab = { opener: {} as unknown, location: { replace: (url: string) => browser.tabs.push(url) } };
      return tab;
    },
    location: { assign: (url: string) => browser.downloads.push(url) },
  });
  try {
    run(browser);
  } finally {
    globals.forEach((name, index) => {
      const descriptor = previous[index];
      if (descriptor) Object.defineProperty(globalThis, name, descriptor);
      else Reflect.deleteProperty(globalThis, name);
    });
  }
}

const request = {
  projectId: "p1",
  chatId: "c1",
  cwd: "/workspace",
  path: "/workspace/src/app.go",
};

test("application files use scoped openers, media previews and download fallback", () => {
  const dispose = fileOpenerStore
    .getState()
    .register("test-editor", ["p1"], () => "https://editor.example/file");
  try {
    withBrowser({}, (browser) => {
      assert.equal(openWorkspaceFile(request), "editor");
      assert.deepEqual(browser.tabs, ["https://editor.example/file"]);
      assert.equal(openWorkspaceFile({ ...request, projectId: "p2" }), "download");
      assert.deepEqual(browser.downloads, [
        "/api/chats/c1/files/download?path=src%2Fapp.go",
      ]);
      assert.equal(openWorkspaceFile({ ...request, path: "/workspace/photo.png" }), "media");
      assert.deepEqual(mediaViewerStore.getState().item, {
        name: "photo.png",
        kind: "image",
        url: "/api/chats/c1/media-open?path=%2Fworkspace%2Fphoto.png",
      });
      assert.equal(openWorkspaceFile({ ...request, path: "/workspace/archive.zip" }), "download");
      for (const path of ["/etc/passwd", "/workspace/../secret", "/workspace/"])
        assert.equal(openWorkspaceFile({ ...request, path }), "ignored");
      assert.equal(browser.tabs.length, 1);
      assert.equal(browser.downloads.length, 2);
    });
  } finally {
    dispose();
    mediaViewerStore.getState().close();
  }
});

test("an editor inside Remote's page claims the file as a link click", () => {
  const dispose = fileOpenerStore
    .getState()
    .register("scratch", ["p1"], ({ path }) => `/api/chats/c1/ide-open?scratch=1&path=${encodeURIComponent(path)}`);
  try {
    withBrowser({ claims: (url) => url.includes("ide-open") }, (browser) => {
      assert.equal(openWorkspaceFile(request), "editor");
      assert.deepEqual(browser.offered, ["/api/chats/c1/ide-open?scratch=1&path=%2Fworkspace%2Fsrc%2Fapp.go"]);
      assert.deepEqual(browser.tabs, []);
      assert.deepEqual(browser.downloads, []);
    });
  } finally {
    dispose();
  }
});

test("a refused editor tab is reported so the caller can offer a link", () => {
  const dispose = fileOpenerStore
    .getState()
    .register("test-editor", ["p1"], () => "https://editor.example/file");
  try {
    withBrowser({ blocksTabs: true }, (browser) => {
      assert.equal(openWorkspaceFile(request), "blocked");
      assert.deepEqual(browser.downloads, []);
    });
  } finally {
    dispose();
  }
});

test("the editor URL is only resolved for editable workspace files", () => {
  const dispose = fileOpenerStore
    .getState()
    .register("test-editor", ["p1"], () => "https://editor.example/file");
  try {
    assert.equal(workspaceFileEditorUrl(request), "https://editor.example/file");
    assert.equal(workspaceFileEditorUrl({ ...request, projectId: "p2" }), null);
    assert.equal(workspaceFileEditorUrl({ ...request, path: "/workspace/photo.png" }), null);
    assert.equal(workspaceFileEditorUrl({ ...request, path: "/workspace/archive.zip" }), null);
    assert.equal(workspaceFileEditorUrl({ ...request, path: "/workspace/../secret" }), null);
  } finally {
    dispose();
  }
});
