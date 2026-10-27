import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
import { fileIdeUrl } from '../ui/scripts/editorUrls.js';

// Execute the exact page published by the installer on the application origin.
const html = fs.readFileSync(new URL('./remote-open.html', import.meta.url), 'utf8');
const script = html.split('<script>')[1].split('</script>')[0];
function openFile(url) {
  let destination;
  const message = { textContent: '' };
  vm.runInNewContext(script, {
    URL,
    location: { href: url.href, origin: url.origin, host: url.host, replace: value => { destination = value; } },
    document: { querySelector: () => message },
  });
  return { destination: destination && new URL(destination), message: message.textContent };
}

test('file links preserve paths and positions on the current project subdomain', () => {
  for (const slug of ['gamerhead', 'other-project']) {
    const landing = new URL(fileIdeUrl({
      cwd: `/var/lib/remote/projects/${slug}/workspace/src`,
      path: '/workspace/src/a #ü%.ts', line: 12, column: 3,
    }, true, 'code', 'https://remote.example.test'));
    assert.equal(landing.origin, `https://code--${slug}.remote.example.test`);
    const { destination } = openFile(landing);
    assert.equal(destination.origin, landing.origin);
    assert.equal(destination.pathname, '/');
    assert.equal(destination.searchParams.get('folder'), '/workspace/src');
    assert.deepEqual(JSON.parse(destination.searchParams.get('payload')), [
      ['openFile', `vscode-remote://${landing.host}/workspace/src/a%20%23%C3%BC%25.ts:12:3`],
      ['gotoLineMode', 'true'],
    ]);
  }
});

test('files without positions open on the app origin and keep a non-default port', () => {
  const { destination } = openFile(new URL('https://code--gamerhead.remote.test:8443/_static/remote-open.html?file=%2Fworkspace%2FREADME.md'));
  assert.equal(destination.origin, 'https://code--gamerhead.remote.test:8443');
  assert.deepEqual(JSON.parse(destination.searchParams.get('payload')), [
    ['openFile', 'vscode-remote://code--gamerhead.remote.test:8443/workspace/README.md'],
  ]);
});

test('invalid file or folder cannot redirect away from the application', () => {
  for (const query of ['file=/etc/passwd', 'file=https://evil.test/a', 'file=/workspace/a&folder=//evil.test', '']) {
    const result = openFile(new URL(`https://code--gamerhead.remote.test/_static/remote-open.html?${query}`));
    assert.equal(result.destination, undefined);
    assert.equal(result.message, 'Invalid workspace file.');
  }
});
