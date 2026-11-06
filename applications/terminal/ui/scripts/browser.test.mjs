import assert from "node:assert/strict";
import test from "node:test";

// Run through backend/api's TestTerminalBrowser, which serves ui/ and the
// backend routes with a local shell.
const base = process.env.FUTRX_TERMINAL_URL;

const HARNESS = `<!doctype html>
<html><head>
<link rel="stylesheet" href="/ui/style/xterm.css">
<link rel="stylesheet" href="/ui/style/terminal.css">
</head><body style="margin:0;background:#141519">
<div id="drawer" style="width:560px;height:420px"></div>
<script type="module">
  import activate from '/ui/scripts/main.js';
  const url = (path) => '/api/applications/i1/backend/' + path;
  const context = { chatId: 'chat-1', projectId: 'p1', cwd: '/var/lib/remote/projects/demo/workspace/api' };
  activate({
    application: { id: 'terminal' },
    backend: {
      instances: [{ instanceId: 'i1', scope: 'project', projectId: 'p1' }],
      url,
      fetch: (path, { method, body, query, signal } = {}) => fetch(
        url(path) + (query ? '?' + new URLSearchParams(query) : ''),
        { method, signal, body: typeof body === 'string' || body === undefined ? body : JSON.stringify(body) },
      ),
    },
    ui: { registerDrawer: (drawer) => {
      window.offered = drawer.when(context);
      window.dispose = drawer.mount(document.querySelector('#drawer'), {
        ...context,
        setStatus: (status) => { window.drawerStatus = status; },
      });
    } },
  });
</script></body></html>`;

async function openDrawer(page) {
  await page.route(`${base}/`, (route) => route.fulfill({ contentType: "text/html", body: HARNESS }));
  await page.goto(`${base}/`);
  await page.waitForFunction(() => window.drawerStatus?.label === "Connected - /workspace/api");
  assert.equal(await page.evaluate(() => window.drawerStatus.active), true);
  await page.locator(".xterm-helper-textarea").focus();
}

async function screen(page, text) {
  await page.waitForFunction(
    (expected) => document.querySelector(".xterm-rows")?.textContent.includes(expected),
    text,
    { timeout: 10000 },
  );
}

async function run(page, line) {
  await page.keyboard.type(line);
  await page.keyboard.press("Enter");
}

test("the drawer types into a shell, survives a reload and restarts after exit", { skip: !base }, async () => {
  const { chromium } = await import("playwright");
  const browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] });
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await openDrawer(page);
    assert.equal(await page.evaluate(() => window.offered), true);
    await run(page, "MARKER=kept; echo he''llo");
    await screen(page, "hello");
    await run(page, "echo si''ze=$(stty size)");
    await screen(page, "size=");
    const size = await page.locator(".xterm-rows").textContent();
    assert.doesNotMatch(size, /size=0 0/);

    // A reload is a new viewer of the same shell: its output is replayed and
    // its variables are still set.
    await page.reload();
    await page.waitForFunction(() => window.drawerStatus?.label === "Connected - /workspace/api");
    await screen(page, "hello");
    await page.locator(".xterm-helper-textarea").focus();
    await run(page, "echo va''lue=$MARKER");
    await screen(page, "value=kept");

    // Find opens over the terminal and counts matches in its buffer.
    await page.keyboard.press("Control+f");
    await page.getByRole("textbox", { name: "Find in terminal" }).fill("value=kept");
    await page.locator(".find-status", { hasText: /of/ }).waitFor();
    await page.keyboard.press("Escape");
    assert.equal(await page.locator(".find").isHidden(), true);

    await page.locator(".xterm-helper-textarea").focus();
    await run(page, "exit");
    await screen(page, "Shell exited. Press Enter to start a new one.");
    await page.waitForFunction(() => window.drawerStatus?.label === "Closed - /workspace/api");
    await page.keyboard.press("Enter");
    await page.waitForFunction(() => window.drawerStatus?.label === "Connected - /workspace/api");
    await run(page, "echo va''lue=[$MARKER]");
    await screen(page, "value=[]");

    await page.evaluate(() => window.dispose());
    assert.equal(await page.locator(".remote-terminal").count(), 0);
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
