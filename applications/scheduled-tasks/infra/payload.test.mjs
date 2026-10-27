import assert from 'node:assert/strict';
import { execFile, execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { promisify } from 'node:util';
import test from 'node:test';

const cli = new URL('./remote-schedule', import.meta.url).pathname;
const execute = promisify(execFile);

test('shipped CLI payload equals its reviewable source', () => {
  const archive = new URL('./payload.tar.gz', import.meta.url);
  const source = readFileSync(cli);
  const shipped = execFileSync('tar', ['-xOf', archive.pathname, 'infra/remote-schedule']);
  assert.deepEqual(shipped, source);
});

test('CLI creates and resumes through its authenticated runtime endpoint', async () => {
  const requests = [];
  const server = createServer(async (request, response) => {
    let body = '';
    for await (const chunk of request) body += chunk;
    requests.push({ method: request.method, path: request.url, authorization: request.headers.authorization, body: JSON.parse(body) });
    response.setHeader('Content-Type', 'application/json');
    response.end(JSON.stringify({ id: 'task', enabled: true }));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  try {
    const env = { ...process.env, REMOTE_APPLICATION_API: `http://127.0.0.1:${server.address().port}/agent-api/applications`, REMOTE_APPLICATION_GRANT: 'test-grant' };
    const result = await execute('bash', [cli, 'create', '--name', 'Reminder', '--prompt', 'Remember this', '--at', '2030-01-01T12:00:00Z'], { env });
    assert.equal(JSON.parse(result.stdout).enabled, true);
    await execute('bash', [cli, 'resume', 'task'], { env });
    assert.equal(requests[0].method, 'POST');
    assert.equal(requests[0].path, '/agent-api/applications/scheduled-tasks/tasks');
    assert.equal(requests[0].authorization, 'Bearer test-grant');
    assert.equal(requests[0].body.kind, 'once');
    assert.equal(requests[0].body.prompt, 'Remember this');
    assert.deepEqual(requests[1], { method: 'PATCH', path: '/agent-api/applications/scheduled-tasks/tasks/task', authorization: 'Bearer test-grant', body: { enabled: true } });
  } finally {
    await new Promise(resolve => server.close(resolve));
  }
});

test('CLI explains unavailable runtime access without calling an endpoint', async () => {
  await assert.rejects(execute('bash', [cli, 'list'], { env: { ...process.env, REMOTE_APPLICATION_API: '', REMOTE_APPLICATION_GRANT: '' } }), error => error.code === 2 && error.stderr.includes('REMOTE_APPLICATION_API is not set'));
});
