// Private stdio transport for the pinned Kimi REST/WebSocket server. The
// server and its bearer token stay inside the host/container running Kimi.
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { once } from 'node:events';

// Importing the transport creates no processes, timers, or sockets. The Go
// command composition supplies settings and starts one private bridge.
export function startKimiBridge(settings) {
  const send = value => process.stdout.write(JSON.stringify(value) + '\n');
  const child = spawn('kimi', ['web', '--no-open', '--host', '127.0.0.1', '--port', '0', '--log-level', 'error'], {
    stdio: ['ignore', 'pipe', 'pipe'], detached: true,
  });
  let origin, token, socket, closing = false, stderr = '';
  const pending = new Map();
  const scrub = text => token ? String(text).replaceAll(token, '[redacted]') : String(text);
  child.stderr.on('data', data => { stderr = (stderr + data).slice(-settings.stderrTailBytes); });
  child.on('error', error => { send({ type: 'fatal', message: error.message }); shutdown(); });
  child.on('exit', (code, signal) => {
    if (!closing) send({ type: 'fatal', message: scrub(`Kimi server exited (${signal ?? code}): ${stderr}`) });
    shutdown();
  });

  async function shutdown() {
    if (closing) return;
    closing = true;
    clearTimeout(startupTimer);
    socket?.close();
    const exited = child.exitCode !== null || child.signalCode !== null;
    if (!exited && child.pid) {
      const wait = once(child, 'exit').catch(() => {});
      try { process.kill(-child.pid, 'SIGTERM'); } catch { child.kill('SIGTERM'); }
      const killTimer = setTimeout(() => {
        try { process.kill(-child.pid, 'SIGKILL'); } catch {}
      }, settings.shutdownTimeoutMs);
      await wait;
      clearTimeout(killTimer);
    }
    // A crashed server can leave processes in its group even after it exits.
    if (child.pid) { try { process.kill(-child.pid, 'SIGKILL'); } catch {} }
    process.exit(0);
  }
  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);
  const input = createInterface({ input: process.stdin });
  input.on('close', shutdown);
  input.on('line', line => {
    let request;
    try { request = JSON.parse(line); } catch { send({ type: 'fatal', message: 'Invalid Kimi bridge request' }); shutdown(); return; }
    handle(request).catch(error => send({ type: 'response', id: request.id, code: -1, msg: scrub(error.message) }));
  });

  async function handle(request) {
    if (request.type === 'close') { await shutdown(); return; }
    if (!origin || !socket || socket.readyState !== WebSocket.OPEN) throw new Error('Kimi server is not ready');
    if (request.type === 'subscribe') {
      pending.set(String(request.id), request.id);
      socket.send(JSON.stringify({ type: 'subscribe', id: String(request.id), payload: request.payload }));
      return;
    }
    if (request.type !== 'http' || !/^\/api\/v[12]\//.test(request.path)) throw new Error('Invalid Kimi API request');
    const response = await fetch(new URL(request.path, origin), {
      method: request.method ?? 'GET',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: request.body === undefined ? undefined : JSON.stringify(request.body),
      signal: AbortSignal.timeout(settings.requestTimeoutMs), redirect: 'error',
    });
    const text = await response.text();
    let envelope;
    try { envelope = JSON.parse(text); } catch { throw new Error(`Kimi API returned HTTP ${response.status}: ${text.slice(0,2000)}`); }
    send({ type: 'response', id: request.id, code: envelope.code ?? (response.ok ? 0 : response.status), msg: envelope.msg, data: envelope.data });
  }

  const startupTimer = setTimeout(() => {
    send({ type: 'fatal', message: scrub(`Kimi server did not become ready: ${stderr}`) });
    shutdown();
  }, settings.startupTimeoutMs);
  createInterface({ input: child.stdout }).on('line', line => {
    if (origin) return;
    const match = line.match(/Kimi server: (http:\/\/127\.0\.0\.1:\d+\/[^\s]*)/);
    if (!match) return;
    const url = new URL(match[1]);
    token = new URLSearchParams(url.hash.slice(1)).get('token');
    if (!token) { send({ type: 'fatal', message: 'Kimi server did not provide authenticated access' }); shutdown(); return; }
    origin = url.origin;
    socket = new WebSocket(origin.replace('http:', 'ws:') + '/api/v1/ws', [`kimi-code.bearer.${token}`]);
    socket.addEventListener('open', () => { clearTimeout(startupTimer); send({ type: 'ready' }); });
    socket.addEventListener('message', event => {
      let frame;
      try { frame = JSON.parse(event.data); } catch {
        send({ type: 'fatal', message: 'Invalid Kimi event frame' }); shutdown(); return;
      }
      if (frame.type === 'context.spliced') {
        frame.payload = { type: 'context.spliced', agentId: frame.payload?.agentId };
      }
      if (frame.type === 'ack' && pending.has(String(frame.id))) {
        const id = pending.get(String(frame.id)); pending.delete(String(frame.id));
        send({ type: 'response', id, code: frame.code, msg: frame.msg, data: frame.payload });
      } else if (frame.type !== 'server_hello') send({ type: 'event', event: frame });
    });
    socket.addEventListener('close', () => {
      if (!closing) { send({ type: 'fatal', message: 'Kimi event connection closed; run interrupted' }); shutdown(); }
    });
    socket.addEventListener('error', () => {
      if (!closing) { send({ type: 'fatal', message: 'Kimi event connection failed' }); shutdown(); }
    });
  });
}
