// /workspace/scripts/browser.mjs — generic authenticated-browser CLI for
// every futrx project. Reads /workspace/.agents/browser-auth.json to figure
// out which cookies to attach, then drives Playwright.
//
// USAGE
//   node /workspace/scripts/browser.mjs screenshot <url> [--out <path>] [--full]
//   node /workspace/scripts/browser.mjs record     <url> [--duration <ms>] [--out <path>]
//   node /workspace/scripts/browser.mjs run        <recipe.mjs> [--record] [--out <path>] [--timeout <ms>]
//   node /workspace/scripts/browser.mjs connect    [recipe.mjs] [--timeout <ms>]
//
//   --out         override the output file path (default /workspace/.browser/<ts>.<ext>)
//   --full        full-page screenshot (default: viewport only)
//   --duration    record duration in ms (default 5000)
//   --record      (run) record a video of the recipe execution
//   --timeout     (run) abort the recipe after this many ms (default 300000)
//
// CONFIG (/workspace/.agents/browser-auth.json)
//   {
//     "<request-host>": {
//       "cookies": [
//         { "name": "<cookie-name>", "domain": "<cookie-domain>", "secret": "<ENV_VAR>",
//           "path": "/", "httpOnly": true, "secure": true, "sameSite": "None" }
//       ],
//       "basicAuth": "<ENV_VAR>"   // optional; env value is "user:pass".
//                                  // For sites behind HTTP Basic auth (e.g. a
//                                  // pre-launch gate). Applied via Playwright
//                                  // httpCredentials.
//     }
//   }
//
// Match rules for "<request-host>":
//   - exact host match (e.g. "app.graphixy.ai")
//   - wildcard "*.example.com" matches any sub.example.com
//   - "default" matches anything not otherwise listed (skip if you don't want
//     fallback)
//
// MISSING-COOKIE BEHAVIOUR
//   For screenshot/record:
//     - URL host has an entry, AND its named secret is set  → cookies attached.
//     - URL host has an entry, AND its named secret is unset → fail loud
//       (we don't silently take a logged-out shot of an app the user
//       expected us to be authenticated to). The error tells the agent
//       which secret to ask the user for.
//     - URL host has NO entry → assume public, proceed with no cookies.
//       Stdout prints the screenshot/video path normally.
//
//   For run: every cookie from every entry whose secret IS set gets
//   attached up-front, so recipes that visit multiple sites just work.
//   Entries whose secret env var is unset are silently skipped — if the
//   recipe hits a logged-out page that's a recipe-level concern.
//
// RECIPE SHAPE (for `run`)
//   // /workspace/.browser/recipes/<name>.mjs
//   export default async function (page, context) {
//     await page.goto('https://app.example.com/dashboard');
//     await page.click('text=Reports');
//     await page.waitForTimeout(1500);
//     // ...
//   };
//
//   The recipe gets a clean Playwright `page` and `context` with cookies
//   already attached. Anything it returns is printed as JSON; thrown
//   errors abort the run but the video (if any) is still flushed.
//
// OUTPUT — written to /workspace/.browser/ (override with $BROWSER_OUT_DIR).
//   Output path is printed on stdout so callers can `Read` the file.

import { mkdir, readFile, rename, writeFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { dirname, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const CONFIG_PATH = process.env.BROWSER_AUTH_CONFIG || '/workspace/.agents/browser-auth.json';
const OUT_DIR = process.env.BROWSER_OUT_DIR || '/workspace/.browser';
const VIEWPORT = { width: 1280, height: 720 };

function die(code, msg) {
  process.stderr.write(msg + '\n');
  process.exit(code);
}

function flag(args, name) {
  const i = args.indexOf(`--${name}`);
  return i >= 0 ? args[i + 1] : undefined;
}
function hasFlag(args, name) {
  return args.includes(`--${name}`);
}

function usage() {
  die(2, [
    'usage:',
    '  node /workspace/scripts/browser.mjs screenshot <url> [--out <path>] [--full]',
    '  node /workspace/scripts/browser.mjs record     <url> [--duration <ms>] [--out <path>]',
    '  node /workspace/scripts/browser.mjs run        <recipe.mjs> [--record] [--out <path>] [--timeout <ms>]',
    '  node /workspace/scripts/browser.mjs connect    [recipe.mjs] [--timeout <ms>]',
    '',
    `config: ${CONFIG_PATH}`,
    `output: ${OUT_DIR} (override with $BROWSER_OUT_DIR)`,
  ].join('\n'));
}

const args = process.argv.slice(2);
const [cmd, posArg, ...rest] = args;
if (!cmd) usage();
if (!['screenshot', 'record', 'run', 'connect'].includes(cmd)) usage();
// connect's recipe arg is optional (no recipe = report open tabs); the
// other commands all require their positional arg.
if (cmd !== 'connect' && !posArg) usage();

let url;
if (cmd === 'screenshot' || cmd === 'record') {
  try {
    url = new URL(posArg);
  } catch {
    die(2, `not a valid URL: ${posArg}`);
  }
}

// --- Load Playwright -----------------------------------------------------
// Playwright may live in any of:
//   - /workspace/node_modules/playwright            (workspace-level install)
//   - /workspace/<project>/node_modules/playwright  (project-level install,
//     common when the project itself uses Playwright for E2E)
//   - global node_modules                            (cli-installed)
//
// We try direct ESM import first (works when cwd's node_modules has it or
// it's globally installed); on failure, walk /workspace/*/ and use
// createRequire to leverage Node's full CJS resolution + package.json
// "main"/"exports" handling (the package's ESM entry exposes `chromium`
// directly; the CJS entry exposes it via `.default.chromium`).
async function loadChromium() {
  try {
    return (await import('playwright')).chromium;
  } catch {}
  const { readdir } = await import('node:fs/promises');
  let entries = [];
  try {
    entries = await readdir('/workspace', { withFileTypes: true });
  } catch {}
  for (const e of entries) {
    if (!e.isDirectory()) continue;
    const pkgJson = `/workspace/${e.name}/node_modules/playwright/package.json`;
    if (!existsSync(pkgJson)) continue;
    try {
      const requireFrom = createRequire(`/workspace/${e.name}/package.json`);
      const mod = requireFrom('playwright');
      return mod.chromium || mod.default?.chromium;
    } catch {}
  }
  return null;
}

const chromium = await loadChromium();
if (!chromium) {
  die(4, [
    'playwright is not installed in this workspace.',
    '',
    'Either install at workspace level (recommended, available to every',
    'project script):',
    '  cd /workspace && npm init -y >/dev/null && npm install --save-dev playwright',
    '  npx playwright install chromium',
    '',
    'Or install inside one of your project subdirs and re-run; this script',
    'discovers /workspace/*/node_modules/playwright automatically.',
    '',
    'First install downloads ~200MB of Chromium; cached for subsequent runs.',
  ].join('\n'));
}

// The project's Playwright driver must always launch the patched browser.
const EXPECTED_CHROME_VERSION = '__PW_CFT_VERSION__';
const CHROME = '__BROWSER_DIR__/chrome';
if (cmd !== 'connect') {
  let version = '';
  try { version = execFileSync(CHROME, ['--version'], { encoding: 'utf8', timeout: 5000 }).trim(); } catch {}
  if (![`Google Chrome for Testing ${EXPECTED_CHROME_VERSION}`, `Chromium ${EXPECTED_CHROME_VERSION}`].includes(version)) {
    die(4, `Open the Browser pane to install Chrome ${EXPECTED_CHROME_VERSION}, then retry.`);
  }
}

// --- connect: drive the live GUI browser the user logged into ------------
// Attaches to the persistent headed Chrome over CDP (the same session the
// user logged into through the noVNC view) and runs an agent recipe against
// it, or — with no recipe — reports the open tabs so the agent can see the
// session state. Unlike the headless commands it does NOT attach cookies
// from browser-auth.json: the live profile is already authenticated.
// Disconnecting leaves the browser running for the human.
//
// Write policy (v1): before any public or irreversible action (post, reply,
// DM, follow, purchase, settings change) the agent must confirm with the
// user first. The human can also watch and intervene through the noVNC view.
if (cmd === 'connect') {
  const cdpURL = process.env.BROWSER_CDP_URL || 'http://127.0.0.1:9222';
  let browser;
  try {
    browser = await chromium.connectOverCDP(cdpURL);
  } catch {
    die(6, [
      `could not connect to the GUI browser at ${cdpURL}.`,
      '',
      'Ask the user to open the Browser pane (that starts the session) and,',
      'for authenticated sites, log in first; then retry.',
    ].join('\n'));
  }
  if (browser.version() !== EXPECTED_CHROME_VERSION) {
    await browser.close();
    die(4, 'The live browser is outdated. Reopen the Browser pane to update it.');
  }
  const context = browser.contexts()[0] || (await browser.newContext());
  let exitCode = 0;
  try {
    if (posArg) {
      const recipePath = resolve(posArg);
      if (!existsSync(recipePath)) die(2, `recipe not found: ${recipePath}`);
      let mod;
      try {
        mod = await import(pathToFileURL(recipePath).href);
      } catch (err) {
        die(9, `failed to load recipe ${recipePath}: ${err.message}`);
      }
      if (typeof mod.default !== 'function') {
        die(9, `recipe ${recipePath} must export a default async function (page, context)`);
      }
      const page = context.pages()[0] || (await context.newPage());
      const timeoutMs = parseInt(flag(rest, 'timeout') || '300000', 10);
      const result = await Promise.race([
        mod.default(page, context),
        new Promise((_, reject) =>
          setTimeout(() => reject(new Error(`recipe exceeded --timeout ${timeoutMs}ms`)), timeoutMs),
        ),
      ]);
      if (result !== undefined) {
        try {
          process.stdout.write(JSON.stringify(result) + '\n');
        } catch {
          process.stdout.write(String(result) + '\n');
        }
      }
    } else {
      const tabs = [];
      for (const pg of context.pages()) {
        tabs.push({ url: pg.url(), title: await pg.title().catch(() => '') });
      }
      process.stdout.write(JSON.stringify({ cdp: cdpURL, tabs }) + '\n');
    }
  } catch (err) {
    process.stderr.write(`error: ${err.message}\n`);
    if (err.stack) process.stderr.write(err.stack + '\n');
    exitCode = 1;
  } finally {
    await browser.close(); // disconnects CDP; leaves the GUI browser running
  }
  process.exit(exitCode);
}

// --- Load + match config -------------------------------------------------
let config = {};
if (existsSync(CONFIG_PATH)) {
  try {
    config = JSON.parse(await readFile(CONFIG_PATH, 'utf8'));
  } catch (err) {
    die(5, `failed to parse ${CONFIG_PATH}: ${err.message}`);
  }
} else {
  // Create an empty config so the agent has a file to add to.
  await mkdir(dirname(CONFIG_PATH), { recursive: true });
  await writeFile(CONFIG_PATH, '{}\n');
}

function pickEntry(host) {
  if (config[host]) return config[host];
  for (const key of Object.keys(config)) {
    if (key.startsWith('*.')) {
      const suffix = key.slice(1); // ".example.com"
      if (host.endsWith(suffix) && host !== suffix.slice(1)) return config[key];
    }
  }
  return config.default;
}

function cookieFromEntry(c, fallbackHost) {
  return {
    name: c.name,
    value: process.env[c.secret],
    domain: c.domain || fallbackHost,
    path: c.path || '/',
    httpOnly: c.httpOnly !== false,
    secure: c.secure !== false,
    sameSite: c.sameSite || 'None',
    ...(c.expires != null ? { expires: c.expires } : {}),
  };
}

// HTTP Basic auth: an entry may carry `"basicAuth": "<ENV_VAR>"` whose value
// is "user:pass". Used for pre-launch sites behind a Cloudflare/edge gate.
// Playwright applies this at the context level via httpCredentials.
function basicAuthFromEntry(entry, origin) {
  if (!entry?.basicAuth) return undefined;
  const raw = process.env[entry.basicAuth];
  if (!raw) return undefined;
  const i = raw.indexOf(':');
  if (i < 0) return undefined;
  return { username: raw.slice(0, i), password: raw.slice(i + 1), origin };
}

let cookies = [];
let httpCredentials;
if (cmd === 'screenshot' || cmd === 'record') {
  // URL-driven: if this host has an entry, every cookie's secret must be
  // set (fail loud — agent should not silently take a logged-out shot of
  // an app that's supposed to be authenticated). If the host has no
  // entry, assume public and proceed with no cookies; the agent can tell
  // from the AUTH line in the output that nothing was attached.
  const entry = pickEntry(url.host);
  if (entry) {
    cookies = (entry.cookies || []).map((c) => {
      if (!c.secret) {
        die(7, `${CONFIG_PATH}: cookie for ${url.host} is missing "secret" (the env-var name holding the cookie value)`);
      }
      if (!process.env[c.secret]) {
        die(8, [
          `secret ${c.secret} is not set in the environment.`,
          '',
          `Ask the user to add it via the project Containers → Secrets UI.`,
          `Tell them which cookie to copy: ${c.name} from ${c.domain}.`,
        ].join('\n'));
      }
      return cookieFromEntry(c, url.host);
    });
    httpCredentials = basicAuthFromEntry(entry, url.origin);
  }
  // No entry → public site / not-configured-yet; proceed with cookies = [].
} else if (cmd === 'run') {
  // Recipe-driven: we don't know up-front which sites the recipe will
  // visit, so attach every cookie whose secret is set. Skip the rest
  // silently — if the recipe needs them, it'll hit a logged-out page and
  // can surface its own error.
  for (const host of Object.keys(config)) {
    const entry = config[host];
    if (entry?.cookies) {
      for (const c of entry.cookies) {
        if (!c.secret || !process.env[c.secret]) continue;
        cookies.push(cookieFromEntry(c, host.startsWith('*.') ? host.slice(2) : host));
      }
    }
    // First entry with a usable basicAuth wins (context-level, no origin so it
    // satisfies whichever gated host the recipe visits).
    if (!httpCredentials && entry?.basicAuth && process.env[entry.basicAuth]) {
      httpCredentials = basicAuthFromEntry(entry, undefined);
    }
  }
}

// --- Launch + drive Playwright ------------------------------------------
await mkdir(OUT_DIR, { recursive: true });

const recordingEnabled = cmd === 'record' || (cmd === 'run' && hasFlag(rest, 'record'));

const launchOpts = { headless: true, executablePath: CHROME };
const contextOpts = { viewport: VIEWPORT };
if (recordingEnabled) {
  contextOpts.recordVideo = { dir: OUT_DIR, size: VIEWPORT };
}
if (httpCredentials) {
  contextOpts.httpCredentials = httpCredentials;
}

// For `run`, resolve the recipe up-front so we fail fast on bad path
// before launching Chromium.
let recipeModule;
if (cmd === 'run') {
  const recipePath = resolve(posArg);
  if (!existsSync(recipePath)) die(2, `recipe not found: ${recipePath}`);
  try {
    recipeModule = await import(pathToFileURL(recipePath).href);
  } catch (err) {
    die(9, `failed to load recipe ${recipePath}: ${err.message}`);
  }
  if (typeof recipeModule.default !== 'function') {
    die(9, `recipe ${recipePath} must export a default async function (page, context)`);
  }
}

const browser = await chromium.launch(launchOpts);
const context = await browser.newContext(contextOpts);
if (cookies.length) await context.addCookies(cookies);
const page = await context.newPage();

let exitCode = 0;
let recordOverride;
try {
  if (cmd === 'screenshot') {
    await page.goto(url.toString(), { waitUntil: 'networkidle', timeout: 30_000 });
    const out = resolve(flag(rest, 'out') || `${OUT_DIR}/screenshot-${ts()}.png`);
    await mkdir(dirname(out), { recursive: true });
    await page.screenshot({ path: out, fullPage: hasFlag(rest, 'full') });
    process.stdout.write(out + '\n');
  } else if (cmd === 'record') {
    const duration = parseInt(flag(rest, 'duration') || '5000', 10);
    await page.goto(url.toString(), { timeout: 30_000 });
    await page.waitForTimeout(duration);
    recordOverride = flag(rest, 'out');
  } else if (cmd === 'run') {
    const timeoutMs = parseInt(flag(rest, 'timeout') || '300000', 10);
    recordOverride = flag(rest, 'out');
    const result = await Promise.race([
      recipeModule.default(page, context),
      new Promise((_, reject) =>
        setTimeout(() => reject(new Error(`recipe exceeded --timeout ${timeoutMs}ms`)), timeoutMs),
      ),
    ]);
    if (result !== undefined) {
      // Recipes can return data (e.g. scraped text). Print as JSON so
      // callers can parse stdout. Video path (if any) prints separately
      // in the finally block.
      try {
        process.stdout.write(JSON.stringify(result) + '\n');
      } catch {
        process.stdout.write(String(result) + '\n');
      }
    }
  }
} catch (err) {
  process.stderr.write(`error: ${err.message}\n`);
  if (err.stack) process.stderr.write(err.stack + '\n');
  exitCode = 1;
} finally {
  const video = recordingEnabled ? page.video() : null;
  await page.close();
  await context.close();
  await browser.close();
  if (video) {
    const defaultPath = await video.path();
    if (recordOverride) {
      const target = resolve(recordOverride);
      await mkdir(dirname(target), { recursive: true });
      await rename(defaultPath, target);
      process.stdout.write(target + '\n');
    } else {
      process.stdout.write(defaultPath + '\n');
    }
  }
}

process.exit(exitCode);

function ts() {
  const d = new Date();
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`;
}
