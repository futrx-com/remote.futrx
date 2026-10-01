import assert from 'node:assert/strict';
import { chromium } from 'playwright';

let input = '';
for await (const chunk of process.stdin) input += chunk;
const { base, app, launch, cookie } = JSON.parse(input);
const browser = await chromium.launch({
  headless: true,
  args: ['--no-sandbox', '--no-proxy-server', '--host-resolver-rules=MAP remote.test 127.0.0.1, MAP *.remote.test 127.0.0.1'],
});
try {
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  await context.addCookies([{ name: 'remote_session', value: cookie, domain: '.remote.test', path: '/', secure: true, httpOnly: true, sameSite: 'Lax' }]);
  const page = await context.newPage();
  await page.goto(base);
  assert.equal(await page.evaluate(async () => (await fetch('/api/private')).text()), 'PLATFORM_SECRET');
  async function socketResult(url) {
    return page.evaluate(url => new Promise(resolve => {
      const ws = new WebSocket(url);
      const timeout = setTimeout(() => { ws.close(); resolve('timeout'); }, 5000);
      ws.onopen = () => ws.send('hello');
      ws.onmessage = event => { clearTimeout(timeout); ws.close(); resolve(event.data); };
      ws.onerror = () => { clearTimeout(timeout); resolve('blocked'); };
    }), url);
  }
  assert.equal(await socketResult(base.replace('https:', 'wss:') + '/ws/private'), 'hello');
  await page.goto(launch);
  assert.equal(new URL(page.url()).origin, app);
  assert.equal(await page.title(), 'Application fixture');
  assert.equal(await page.evaluate(() => document.cookie.includes('remote_session')), false);
  assert.equal(await socketResult(app.replace('https:', 'wss:') + '/socket'), 'hello');
  assert.equal(await socketResult(base.replace('https:', 'wss:') + '/ws/private'), 'blocked');
  assert.equal(await page.evaluate(async base => {
    try { return await (await fetch(base + '/api/private', { credentials: 'include' })).text(); }
    catch { return 'blocked'; }
  }, base), 'blocked');
  // A simple request bypasses CORS preflight. The server must reject it before
  // any mutation, even though the browser still attaches the shared cookie.
  await page.evaluate(async base => {
    await fetch(base + '/api/private', { method: 'POST', mode: 'no-cors', credentials: 'include', body: 'attempt' });
    await fetch(base + '/auth/logout', { mode: 'no-cors', credentials: 'include' });
  }, base);
  assert.equal(await page.evaluate(async () => (await fetch('/api/private')).text()), '<!doctype html><title>Application fixture</title><p>APPLICATION</p>');
  // Trigger navigation from the application; page.goto simulates typing a URL
  // in the address bar and intentionally carries Sec-Fetch-Site: none.
  const [response] = await Promise.all([
    page.waitForNavigation(),
    page.evaluate(base => { window.location.href = base + '/api/private'; }, base),
  ]);
  assert.equal(response.status(), 403, 'an application must not navigate directly into API routes');
  await page.goto(base);
  const me = await page.evaluate(async () => (await fetch('/auth/me')).json());
  assert.equal(me.authenticated, true, 'cross-origin logout must not revoke the session');
  console.log('Chromium: launch redirect, cookie filtering, own-app requests/WebSocket, and blocked platform reads/writes/logout/WebSocket/navigation passed');
} finally {
  await browser.close();
}
