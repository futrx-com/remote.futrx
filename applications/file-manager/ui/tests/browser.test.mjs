import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// The drawer renders in Remote's page and reaches its backend through
// remote.backend. These checks serve the extension from ui/ and answer its
// backend routes with a fixture workspace.
const skip = process.env.FUTRX_BROWSER_TEST !== "1";
const ORIGIN = "http://remote.test";
const BACKEND = "/api/applications/i1/backend/";

const LISTINGS = {
  "": [
    { name: "src", path: "src", isDir: true, modTime: 1 },
    { name: ".env", path: ".env", isDir: false, size: 8, modTime: 1 },
  ],
  src: [{ name: "app.go", path: "src/app.go", isDir: false, size: 12, modTime: 1 }],
};

function harness({ remoteFiles = true, scratch = false } = {}) {
  return `<!doctype html>
<html style="--fg: 12 13 14;--fg-muted: 80 80 80;--fg-subtle: 90 90 90;--fg-faint: 100 100 100;--bg-surface-rgb: 240 241 242;--bg-inset-rgb: 230 231 232;--accent-blue: 20 90 200;--accent-red: 200 20 20;--accent-yellow: 160 120 20;--line: #ccc">
<head><link rel="stylesheet" href="/style/files.css"></head>
<body style="margin:0"><div id="drawer" style="width:100%;max-width:560px;height:600px"></div>
<script type="module">
  import activate from '/scripts/main.js';
  window.opened = [];
  window.claimed = [];
  ${scratch ? `document.addEventListener('click', event => {
    const link = event.target.closest('a');
    if (!link || !/ide-open|remote-open/.test(link.href)) return;
    window.claimed.push(link.href);
    event.preventDefault();
    event.stopImmediatePropagation();
  }, true);` : ""}
  const url = (path) => '${BACKEND}' + path;
  activate({
    application: {},
    backend: {
      instances: [{ instanceId: 'i1', scope: 'project', projectId: 'p1' }],
      url,
      fetch: (path, { method, query, signal } = {}) =>
        fetch(url(path) + (query ? '?' + new URLSearchParams(query) : ''), { method, signal }),
    },
    ${remoteFiles ? `files: {
      open: request => { window.opened.push(request); return window.blockTabs ? 'blocked' : 'editor'; },
      editorUrl: request => 'http://editor.test/open?file=' + encodeURIComponent(request.path),
    },` : ""}
    ui: { registerDrawer: drawer => {
      const context = { chatId: 'c1', projectId: 'p1', cwd: '/var/lib/remote/projects/alpha/workspace' };
      window.visible = drawer.when(context);
      window.dispose = drawer.mount(document.querySelector('#drawer'), {
        ...context,
        setStatus: status => { window.drawerStatus = status; },
        setActions: actions => { window.actions = actions; },
      });
    } },
  });
</script></body></html>`;
}

async function serve(page, options = {}, { listings = LISTINGS, onSearch } = {}) {
  await page.route(`${ORIGIN}/**`, async (route) => {
    const url = new URL(route.request().url());
    const { pathname, searchParams } = url;
    if (/^\/(scripts|style)\/[A-Za-z0-9/]+\.(js|css)$/.test(pathname)) {
      await route.fulfill({
        contentType: pathname.endsWith(".css") ? "text/css" : "text/javascript",
        body: await readFile(new URL(`../${pathname.slice(1)}`, import.meta.url), "utf8"),
      });
    } else if (pathname === `${BACKEND}list`) {
      const entries = listings[searchParams.get("path") ?? ""];
      await route.fulfill(entries
        ? { json: { path: searchParams.get("path") ?? "", entries, truncated: false } }
        : { status: 404, json: { error: "Not found." } });
    } else if (pathname === `${BACKEND}search`) {
      const query = searchParams.get("q");
      if (onSearch && (await onSearch(route, query))) return;
      await route.fulfill({ json: {
        entries: query === "app" ? [LISTINGS.src[0]] : [],
        truncated: false,
      } });
    } else if (pathname === `${BACKEND}archive`) {
      assert.equal(route.request().method(), "POST");
      assert.equal(searchParams.get("path"), "src");
      await route.fulfill({ json: { path: ".file-manager-downloads/0a1b/src.zip" } });
    } else if (pathname === "/api/chats/c1/files/download") {
      // The built ZIP downloads through the same route as any file.
      const file = searchParams.get("path");
      assert.ok(["src/app.go", ".file-manager-downloads/0a1b/src.zip"].includes(file));
      await route.fulfill({
        headers: { "Content-Disposition": `attachment; filename="${file.slice(file.lastIndexOf("/") + 1)}"` },
        body: file.endsWith(".zip") ? "PK" : "package main",
      });
    } else if (pathname === "/api/projects/p1/applications") {
      await route.fulfill({ json: options.code ? [{ applicationId: "code-server", status: "running" }] : [] });
    } else {
      await route.fulfill({ contentType: "text/html", body: harness(options) });
    }
  });
}

async function launch() {
  const { chromium } = await import("playwright");
  return chromium.launch({ headless: true, args: ["--no-sandbox"] });
}

test("browse, search, download and mobile layout", { skip }, async () => {
  const browser = await launch();
  try {
    const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await serve(page);
    await page.goto(ORIGIN);
    assert.equal(await page.evaluate(() => window.visible), true);
    await page.getByRole("button", { name: "src", exact: true }).click();
    await page.getByRole("button", { name: "app.go", exact: true }).waitFor();
    assert.equal(await page.locator(":focus").getAttribute("aria-label"), "src");
    await page.getByRole("button", { name: "src", exact: true }).press("Enter");
    assert.equal(await page.getByRole("button", { name: "app.go", exact: true }).count(), 0);
    await page.getByRole("button", { name: "src", exact: true }).press(" ");
    await page.getByRole("button", { name: "app.go", exact: true }).waitFor();
    await page.getByLabel("Search all files", { exact: true }).fill("app");
    await page.locator(".detail .name", { hasText: "app.go" }).waitFor();
    assert.equal(await page.locator(".detail").count(), 1);
    assert.equal(await page.locator(".detail .dir").textContent(), "src/");
    // Files stream from Remote's chat download route. Chromium reports a routed
    // download from a `download` link only after a long delay, so the link is
    // checked here; the archive below and the legacy test download for real.
    const fileLink = page.getByRole("link", { name: "Download app.go", exact: true });
    assert.equal(await fileLink.getAttribute("href"), "/api/chats/c1/files/download?path=src%2Fapp.go");
    assert.equal(await fileLink.getAttribute("download"), "app.go");
    await page.getByLabel("Search all files", { exact: true }).fill("");
    await page.getByRole("button", { name: "src", exact: true }).waitFor();
    // The tree kept its expanded folder while the search was showing.
    await page.getByRole("button", { name: "app.go", exact: true }).waitFor();
    const [archive] = await Promise.all([
      page.waitForEvent("download"),
      page.getByRole("button", { name: "Download src as zip", exact: true }).click(),
    ]);
    assert.equal(archive.suggestedFilename(), "src.zip");
    assert.equal(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      true,
    );
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});

test("the drawer drives its header, opens files and disposes cleanly", { skip }, async () => {
  const browser = await launch();
  try {
    const page = await browser.newPage();
    await serve(page, {}, {
      listings: { ...LISTINGS, "": [...LISTINGS[""], { name: "escape", path: "../escape", isDir: false }] },
    });
    await page.goto(ORIGIN);
    await page.waitForFunction(() => window.drawerStatus?.label === "workspace");
    assert.equal(await page.evaluate(() => window.drawerStatus.active), undefined);
    assert.deepEqual(
      await page.evaluate(() => window.actions.map(({ id, label, busy }) => ({ id, label, busy }))),
      [{ id: "refresh", label: "Refresh files", busy: false }],
    );
    await page.getByLabel("Search all files", { exact: true }).fill("app");
    await page.waitForFunction(() => window.drawerStatus?.label === "1 result");
    await page.getByLabel("Search all files", { exact: true }).fill("");
    await page.waitForFunction(() => window.drawerStatus?.label === "workspace");
    await page.route(`${ORIGIN}${BACKEND}list`, (route) =>
      route.fulfill({ status: 503, body: "Unavailable" }),
    );
    await page.evaluate(() => window.actions[0].onClick());
    await page.locator(".banner.error", { hasText: "Unable to load files (503)" }).waitFor();
    await page.unroute(`${ORIGIN}${BACKEND}list`);
    await page.evaluate(() => window.actions[0].onClick());
    await page.getByRole("button", { name: "src", exact: true }).click();
    await page.getByRole("button", { name: "app.go", exact: true }).click();
    await page.waitForFunction(() => window.opened.length === 1);
    assert.deepEqual(await page.evaluate(() => window.opened[0]), {
      chatId: "c1",
      projectId: "p1",
      cwd: "/var/lib/remote/projects/alpha/workspace",
      path: "/workspace/src/app.go",
    });
    // A path that leaves the workspace never reaches an opener.
    await page.getByRole("button", { name: "escape", exact: true }).click();
    assert.equal(await page.evaluate(() => window.opened.length), 1);
    // A refused editor tab becomes a link the user can click through.
    await page.context().route("http://editor.test/**", (route) =>
      route.fulfill({ contentType: "text/html", body: "editor" }),
    );
    await page.evaluate(() => { window.blockTabs = true; });
    await page.getByRole("button", { name: "app.go", exact: true }).click();
    const link = page.getByRole("link", { name: "Open app.go", exact: true });
    const [tab] = await Promise.all([page.context().waitForEvent("page"), link.click()]);
    await tab.waitForURL("http://editor.test/**");
    assert.equal(new URL(tab.url()).searchParams.get("file"), "/workspace/src/app.go");
    assert.equal(await tab.evaluate(() => window.opener), null);
    await tab.close();
    await link.waitFor({ state: "detached" });
    assert.equal(await page.evaluate(() => window.opened.length), 2);
    // Colours follow Remote's own theme tokens.
    assert.equal(
      await page.locator(".remote-files").evaluate((root) => getComputedStyle(root).color),
      "rgb(12, 13, 14)",
    );
    await page.evaluate(() => window.dispose());
    assert.equal(await page.locator(".remote-files").count(), 0);
  } finally {
    await browser.close();
  }
});

test("a replaced search cannot overwrite current results, and failures can be cleared", { skip }, async () => {
  const browser = await launch();
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    let release;
    let started;
    let finished;
    const pending = new Promise((resolve) => { release = resolve; });
    const requested = new Promise((resolve) => { started = resolve; });
    const settled = new Promise((resolve) => { finished = resolve; });
    await serve(page, {}, {
      onSearch: async (route, query) => {
        if (query === "slow") {
          started();
          await pending;
          await route.fulfill({ json: { entries: [{ name: "stale.txt", path: "stale.txt" }] } }).catch(() => {});
          finished();
          return true;
        }
        if (query === "broken") {
          await route.fulfill({ status: 503, body: "Unavailable" });
          return true;
        }
        return false;
      },
    });
    await page.goto(ORIGIN);
    const search = page.getByLabel("Search all files", { exact: true });
    await page.getByRole("button", { name: "src", exact: true }).click();
    await page.getByRole("button", { name: "app.go", exact: true }).waitFor();
    await search.fill("slow");
    await requested;
    await search.fill("app");
    await page.locator(".detail .name", { hasText: "app.go" }).waitFor();
    release();
    await settled;
    assert.equal(await page.locator(".detail .name").textContent(), "app.go");
    await search.fill("broken");
    await page.locator(".banner.error").waitFor();
    assert.equal(await page.locator(".banner.error").textContent(), "Unable to load files (503). Try Refresh.");
    await page.getByRole("button", { name: "Clear search", exact: true }).click();
    assert.equal(await search.inputValue(), "");
    assert.equal(await search.evaluate((input) => input === document.activeElement), true);
    await page.getByRole("button", { name: "app.go", exact: true }).waitFor();
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});

test("older Remote prefers Scratch Tab, then Code Server, then downloads", { skip }, async (t) => {
  const browser = await launch();
  try {
    for (const scratch of [true, false]) {
      for (const code of [true, false]) {
        await t.test(`Scratch Tab=${scratch}, Code Server=${code}`, async () => {
          const context = await browser.newContext();
          try {
            const page = await context.newPage();
            const errors = [];
            page.on("pageerror", (error) => errors.push(error.message));
            await context.route("http://code--alpha.remote.test/**", (route) =>
              route.fulfill({ contentType: "text/html", body: "Code Server" }),
            );
            await serve(page, { remoteFiles: false, scratch, code });
            const applications = page.waitForResponse(`${ORIGIN}/api/projects/p1/applications`);
            await page.goto(ORIGIN);
            await (await applications).finished();
            await page.getByRole("button", { name: "src", exact: true }).click();
            const file = page.getByRole("button", { name: "app.go", exact: true });
            if (scratch) {
              await file.click();
              await page.waitForFunction(() => window.claimed.length === 1);
              const link = new URL(await page.evaluate(() => window.claimed[0]));
              assert.equal(link.pathname, code ? "/_static/remote-open.html" : "/api/chats/c1/ide-open");
              assert.equal(link.searchParams.get(code ? "file" : "path"), "/workspace/src/app.go");
              assert.equal(context.pages().length, 1);
            } else if (code) {
              const [tab] = await Promise.all([context.waitForEvent("page"), file.click()]);
              await tab.waitForURL("http://code--alpha.remote.test/**");
              assert.equal(new URL(tab.url()).searchParams.get("file"), "/workspace/src/app.go");
              assert.equal(await tab.evaluate(() => window.opener), null);
            } else {
              const [download] = await Promise.all([page.waitForEvent("download"), file.click()]);
              assert.equal(download.suggestedFilename(), "app.go");
            }
            assert.deepEqual(errors, []);
          } finally {
            await context.close();
          }
        });
      }
    }
  } finally {
    await browser.close();
  }
});
