import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const installer = fs.readFileSync(new URL('./migrate-settings.cjs', import.meta.url), 'utf8');
const writer = fs.readFileSync(new URL('../backend/containerio/write-settings.js', import.meta.url), 'utf8');
function fixture(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'code-server-settings-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const root = path.join(dir, 'workspace', '.remote', 'code-server');
  const active = path.join(dir, 'container', 'User');
  const durable = path.join(root, 'User', 'settings.json');
  function run(script, settings = '{"editor.fontSize":14}', input) {
    script = script.replaceAll('/workspace/.remote/code-server', root)
      .replaceAll('/root/.local/share/code-server/User', active);
    execFileSync(process.execPath, ['-e', script], {
      env: { ...process.env, CODE_SERVER_SETTINGS_JSON: settings, CODE_SERVER_WS_NAME: 'project' },
      input, encoding: 'utf8',
    });
  }
  return { root, active, durable, install: (settings) => run(installer, settings), save: (input) => run(writer, undefined, input) };
}
function put(file, contents) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, contents);
}

test('initial settings seed once; editor atomic saves survive updates and container replacement', t => {
  const f = fixture(t);
  f.install();
  assert.equal(JSON.parse(fs.readFileSync(f.durable))['editor.fontSize'], 14);
  assert.equal(fs.lstatSync(f.active).isSymbolicLink(), true);
  const edited = '{\n // user setting\n "editor.fontSize": 23,\n}\n';
  const temp = path.join(f.active, 'settings.json.tmp');
  put(temp, edited);
  fs.renameSync(temp, path.join(f.active, 'settings.json'));
  f.install('{"editor.fontSize":99}');
  assert.equal(fs.readFileSync(f.durable, 'utf8'), edited);
  fs.rmSync(path.dirname(f.active), { recursive: true });
  f.install('{"editor.fontSize":99}');
  assert.equal(fs.readFileSync(path.join(f.active, 'settings.json'), 'utf8'), edited);
  assert.equal(fs.statSync(f.durable).mode & 0o777, 0o600);
});

test('migration preserves current editor settings over a stale saved copy and keeps keybindings', t => {
  const f = fixture(t);
  const edited = '{"editor.fontSize":24, "custom.setting":true}\n';
  put(path.join(f.active, 'settings.json'), edited);
  put(path.join(f.active, 'keybindings.json'), '[{"key":"ctrl+k"}]');
  put(path.join(f.root, 'settings.json'), '{"editor.fontSize":12}');
  f.install();
  assert.equal(fs.readFileSync(f.durable, 'utf8'), edited);
  assert.equal(fs.readFileSync(path.join(f.root, 'User', 'keybindings.json'), 'utf8'), '[{"key":"ctrl+k"}]');
  assert.equal(fs.existsSync(path.join(f.root, 'settings.json')), false);
});

test('replacement of an old container restores the legacy durable settings', t => {
  const f = fixture(t);
  const edited = '{ // legacy JSONC\n "editor.fontSize":25\n}\n';
  put(path.join(f.root, 'settings.json'), edited);
  f.install();
  assert.equal(fs.readFileSync(f.durable, 'utf8'), edited);
});

test('Remote settings saves write the same durable file used by the editor', t => {
  const f = fixture(t);
  f.install();
  f.save('{"editor.fontSize":26}');
  assert.equal(JSON.parse(fs.readFileSync(f.durable))['editor.fontSize'], 26);
  assert.equal(fs.lstatSync(f.active).isSymbolicLink(), true);
  f.install();
  assert.equal(JSON.parse(fs.readFileSync(f.durable))['editor.fontSize'], 26);
});
