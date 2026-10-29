import assert from "node:assert/strict";
import test from "node:test";

const base = process.env.FUTRX_FILE_MANAGER_URL;
test(
  "browse, search, download and mobile layout",
  { skip: !base },
  async () => {
    const { chromium } = await import("playwright");
    const browser = await chromium.launch({
      headless: true,
      args: ["--no-sandbox"],
    });
    try {
      const page = await browser.newPage({
        viewport: { width: 390, height: 844 },
      });
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.goto(base);
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
      const [download] = await Promise.all([
        page.waitForEvent("download"),
        page
          .getByRole("link", { name: "Download app.go", exact: true })
          .click(),
      ]);
      assert.equal(download.suggestedFilename(), "app.go");
      await page.getByLabel("Search all files", { exact: true }).fill("");
      await page.getByRole("button", { name: "src", exact: true }).waitFor();
      // The tree kept its expanded folder while the search was showing.
      await page.getByRole("button", { name: "app.go", exact: true }).waitFor();
      const [archive] = await Promise.all([
        page.waitForEvent("download"),
        page
          .getByRole("link", { name: "Download src as zip", exact: true })
          .click(),
      ]);
      assert.equal(archive.suggestedFilename(), "src.zip");
      assert.equal(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
        true,
      );
      assert.deepEqual(errors, []);
    } finally {
      await browser.close();
    }
  },
);

test(
  "application drawer drives the header, bridges only its iframe and disposes cleanly",
  { skip: !base },
  async () => {
    const { chromium } = await import("playwright");
    const { readFile } = await import("node:fs/promises");
    const browser = await chromium.launch({
      headless: true,
      args: ["--no-sandbox"],
    });
    try {
      const page = await browser.newPage();
      await page.route("http://remote.test/**", async (route) => {
        const pathname = new URL(route.request().url()).pathname;
        if (/^\/(?:[A-Za-z][A-Za-z0-9]*\/)*[A-Za-z][A-Za-z0-9]*\.js$/.test(pathname)) {
          await route.fulfill({
            contentType: "text/javascript",
            body: await readFile(
              new URL(`../scripts${pathname}`, import.meta.url),
              "utf8",
            ),
          });
          return;
        }
        await route.fulfill({
          contentType: "text/html",
          body: `<!doctype html><html style="--fg: 12 13 14;--fg-muted: 80 80 80;--bg-surface-rgb: 240 241 242;--bg-inset-rgb: 230 231 232;--accent-blue: 20 90 200;--accent-red: 200 20 20;--line: #ccc"><body><div id="drawer" style="width:560px;height:600px"></div><script type="module">
    import activate from '/main.js';
    window.opened=[];
    activate({application:{web:{subdomain:'files'}},files:{open:request=>{window.opened.push(request);return window.blockTabs?'blocked':'editor'},editorUrl:request=>'http://editor.test/open?file='+encodeURIComponent(request.path)},ui:{registerDrawer:drawer=>{
     window.dispose=drawer.mount(document.querySelector('#drawer'),{chatId:'c1',projectId:'p1',cwd:'/var/lib/remote/projects/alpha/workspace',setStatus:status=>{window.drawerStatus=status},setActions:actions=>{window.actions=actions}});
    }}});
   </script></body></html>`,
        });
      });
      await page.route("http://files--alpha.remote.test/**", async (route) => {
        const url = new URL(route.request().url());
        const response = await route.fetch({
          url: base + url.pathname + url.search,
        });
        await route.fulfill({ response });
      });
      await page.goto("http://remote.test/");
      const frame = page.frameLocator("iframe");
      await page.waitForFunction(() => window.drawerStatus?.label === "workspace");
      assert.equal(await page.evaluate(() => window.drawerStatus.active), undefined);
      assert.deepEqual(
        await page.evaluate(() =>
          window.actions.map(({ id, label, busy }) => ({ id, label, busy })),
        ),
        [{ id: "refresh", label: "Refresh files", busy: false }],
      );
      await frame.getByLabel("Search all files", { exact: true }).fill("app");
      await page.waitForFunction(() => window.drawerStatus?.label === "1 result");
      await frame.getByLabel("Search all files", { exact: true }).fill("");
      await page.waitForFunction(() => window.drawerStatus?.label === "workspace");
      await page.route("http://files--alpha.remote.test/api/list", (route) =>
        route.fulfill({ status: 503, body: "Unavailable" }),
      );
      await page.evaluate(() => window.actions[0].onClick());
      await frame.locator(".banner.error", { hasText: "Unable to load" }).waitFor();
      await page.unroute("http://files--alpha.remote.test/api/list");
      await page.evaluate(() => window.actions[0].onClick());
      await frame.getByRole("button", { name: "src", exact: true }).click();
      await frame.getByRole("button", { name: "app.go", exact: true }).click();
      await page.waitForFunction(() => window.opened.length === 1);
      assert.deepEqual(await page.evaluate(() => window.opened[0]), {
        chatId: "c1",
        projectId: "p1",
        cwd: "/var/lib/remote/projects/alpha/workspace",
        path: "/workspace/src/app.go",
      });
      // A refused editor tab becomes a link the user can click through.
      await page.context().route("http://editor.test/**", (route) =>
        route.fulfill({ contentType: "text/html", body: "editor" }),
      );
      await page.evaluate(() => {
        window.blockTabs = true;
      });
      await frame.getByRole("button", { name: "app.go", exact: true }).click();
      const link = frame.getByRole("link", { name: "Open app.go", exact: true });
      const [tab] = await Promise.all([
        page.context().waitForEvent("page"),
        link.click(),
      ]);
      await tab.waitForURL("http://editor.test/**");
      assert.equal(
        new URL(tab.url()).searchParams.get("file"),
        "/workspace/src/app.go",
      );
      assert.equal(await tab.evaluate(() => window.opener), null);
      await tab.close();
      await link.waitFor({ state: "detached" });
      assert.equal(await page.evaluate(() => window.opened.length), 2);
      assert.equal(
        await frame
          .locator("body")
          .evaluate((body) => getComputedStyle(body).color),
        "rgb(12, 13, 14)",
      );
      await page.evaluate(() => {
        const source = document.querySelector("iframe").contentWindow;
        const data = {
          source: "remote-file-manager",
          type: "open",
          path: "src/app.go",
        };
        window.dispatchEvent(
          new MessageEvent("message", {
            source,
            origin: "http://evil.test",
            data,
          }),
        );
        window.dispatchEvent(
          new MessageEvent("message", {
            source,
            origin: "http://files--alpha.remote.test",
            data: { ...data, path: "../secret" },
          }),
        );
      });
      assert.equal(await page.evaluate(() => window.opened.length), 2);
      await page.evaluate(() => window.dispose());
      assert.equal(await page.locator("iframe").count(), 0);
    } finally {
      await browser.close();
    }
  },
);

test("a replaced search cannot overwrite current results, and failures can be cleared", { skip: !base }, async () => {
  const { chromium } = await import("playwright");
  const browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] });
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
    await page.route(`${base}/api/search?q=slow`, async (route) => {
      started();
      await pending;
      await route.fulfill({ json: { entries: [{ name: "stale.txt", path: "stale.txt" }] } }).catch(() => {});
      finished();
    });
    await page.route(`${base}/api/search?q=broken`, (route) =>
      route.fulfill({ status: 503, body: "Unavailable" }),
    );
    await page.goto(base);
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

test("older Remote prefers Scratch Tab, then Code Server, then downloads", { skip: !base }, async (t) => {
  const { chromium } = await import("playwright");
  const { readFile } = await import("node:fs/promises");
  const browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] });
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
            await page.route("http://remote.test/**", async (route) => {
              const pathname = new URL(route.request().url()).pathname;
              if (/^\/(?:[A-Za-z][A-Za-z0-9]*\/)*[A-Za-z][A-Za-z0-9]*\.js$/.test(pathname)) {
                await route.fulfill({ contentType: "text/javascript", body: await readFile(new URL(`../scripts${pathname}`, import.meta.url), "utf8") });
              } else if (pathname === "/api/projects/p1/applications") {
                await route.fulfill({ json: code ? [{ applicationId: "code-server", status: "running" }] : [] });
              } else if (pathname === "/api/chats/c1/files/download") {
                assert.equal(new URL(route.request().url()).searchParams.get("path"), "src/app.go");
                await route.fulfill({ headers: { "Content-Disposition": 'attachment; filename="app.go"' }, body: "package main" });
              } else {
                await route.fulfill({ contentType: "text/html", body: `<!doctype html><div id="drawer" style="height:600px"></div><script type="module">
                  import activate from '/main.js';
                  window.claimed=[];
                  ${scratch ? `document.addEventListener('click', event => {
                    const link=event.target.closest('a');
                    if (!link || !/ide-open|remote-open/.test(link.href)) return;
                    window.claimed.push(link.href);
                    event.preventDefault();
                    event.stopImmediatePropagation();
                  }, true);` : ""}
                  activate({application:{web:{subdomain:'files'}},ui:{registerDrawer:drawer=>{
                    window.dispose=drawer.mount(document.querySelector('#drawer'),{chatId:'c1',projectId:'p1',cwd:'/var/lib/remote/projects/alpha/workspace',setStatus:status=>{window.drawerStatus=status}});
                  }}});
                </script>` });
              }
            });
            await page.route("http://files--alpha.remote.test/**", async (route) => {
              const url = new URL(route.request().url());
              await route.fulfill({ response: await route.fetch({ url: base + url.pathname + url.search }) });
            });
            const applications = page.waitForResponse("http://remote.test/api/projects/p1/applications");
            await page.goto("http://remote.test/");
            await (await applications).finished();
            const frame = page.frameLocator("iframe");
            await frame.getByRole("button", { name: "src", exact: true }).click();
            const file = frame.getByRole("button", { name: "app.go", exact: true });
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
