import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

const source = fs.readFileSync(new URL("./node-runtime.sh", import.meta.url), "utf8");

function writeNode(file, version, abi) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, `#!/bin/sh
case "$*" in
  '-p process.versions.node') echo '${version}' ;;
  '-p process.versions.modules') echo '${abi}' ;;
  '--version') echo 'v${version}' ;;
  *) exit 1 ;;
esac
`, { mode: 0o755 });
}

function fixture(t, { current = "22.22.1", target = "22.23.4", arch = "amd64", abi = "127", replacementAbi = abi, replacementVersion = target } = {}) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "code-server-node-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const root = path.join(dir, "code-server");
  const binary = path.join(root, "lib/node");
  writeNode(binary, current, abi);
  const original = fs.readFileSync(binary);
  const archiveName = `node-v${target}-linux-${arch === "amd64" ? "x64" : "arm64"}`;
  writeNode(path.join(dir, "archive", archiveName, "bin/node"), replacementVersion, replacementAbi);
  const archive = path.join(dir, "node.tar.gz");
  execFileSync("tar", ["-czf", archive, "-C", path.join(dir, "archive"), archiveName]);
  const digest = createHash("sha256").update(fs.readFileSync(archive)).digest("hex");
  const script = path.join(dir, "node-runtime.sh");
  // Keep the production verifier and release selection, substituting only the
  // official archive digests for the deterministic fixture archive's digest.
  fs.writeFileSync(script, source.replace(/[a-f0-9]{64}/g, digest));
  const bin = path.join(dir, "bin");
  fs.mkdirSync(bin);
  fs.writeFileSync(path.join(bin, "curl"), `#!/bin/sh
echo "$*" >> "$NODE_TEST_DOWNLOADS"
[ "\${NODE_TEST_DOWNLOAD_FAIL:-0}" = 0 ] || exit 22
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then cp "$NODE_TEST_ARCHIVE" "$2"; exit; fi
  shift
done
exit 1
`, { mode: 0o755 });
  const downloads = path.join(dir, "downloads");
  const env = { ...process.env, PATH: `${bin}:${process.env.PATH}`, NODE_TEST_ARCHIVE: archive, NODE_TEST_DOWNLOADS: downloads };
  return {
    binary, original, archive, script, root, downloads,
    run(extraEnv = {}) {
      return spawnSync("bash", [script, root, arch], { env: { ...env, ...extraEnv }, encoding: "utf8" });
    },
    assertUnchanged() {
      assert.deepEqual(fs.readFileSync(binary), original);
      assert.deepEqual(fs.readdirSync(path.join(root, "lib")), ["node"]);
    },
  };
}

for (const { current, target, abi } of [
  { current: "22.22.1", target: "22.23.4", abi: "127" },
  { current: "24.21.0", target: "24.22.1", abi: "137" },
]) {
  for (const arch of ["amd64", "arm64"]) {
    test(`patches Node ${current} on ${arch} and is idempotent`, (t) => {
      const f = fixture(t, { current, target, arch, abi });
      const result = f.run();
      assert.equal(result.status, 0, result.stderr);
      assert.equal(execFileSync(f.binary, ["--version"], { encoding: "utf8" }).trim(), `v${target}`);
      assert.match(fs.readFileSync(f.downloads, "utf8"), new RegExp(`/v${target}/node-v${target}-linux-${arch === "amd64" ? "x64" : "arm64"}\\.tar\\.gz`));
      assert.deepEqual(fs.readdirSync(path.join(f.root, "lib")), ["node"]);
      assert.equal(f.run().status, 0);
      assert.equal(fs.readFileSync(f.downloads, "utf8").trim().split("\n").length, 1);
    });
  }
}

for (const current of ["22.23.4", "22.23.5", "24.22.1", "24.22.2"]) {
  test(`keeps supported Node ${current} without downloading`, (t) => {
    const f = fixture(t, { current });
    assert.equal(f.run().status, 0);
    f.assertUnchanged();
    assert.equal(fs.existsSync(f.downloads), false);
  });
}

test("rejects corrupt downloads before extraction and preserves the old runtime", (t) => {
  const f = fixture(t);
  fs.appendFileSync(f.archive, "corrupted");
  const result = f.run();
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /checksum verification failed/);
  f.assertUnchanged();
});

test("download failure preserves the old runtime and cleans staging files", (t) => {
  const f = fixture(t);
  assert.notEqual(f.run({ NODE_TEST_DOWNLOAD_FAIL: "1" }).status, 0);
  f.assertUnchanged();
});

for (const replacement of [{ replacementVersion: "22.23.3" }, { replacementAbi: "137" }]) {
  test(`rejects incompatible replacement ${JSON.stringify(replacement)}`, (t) => {
    const f = fixture(t, replacement);
    const result = f.run();
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /wrong version or native-module ABI/);
    f.assertUnchanged();
  });
}

for (const current of ["20.20.0", "26.12.1", "22.23.4-rc.1", "invalid"]) {
  test(`rejects unsupported or malformed Node ${current} without downloading`, (t) => {
    const f = fixture(t, { current });
    assert.notEqual(f.run().status, 0);
    f.assertUnchanged();
    assert.equal(fs.existsSync(f.downloads), false);
  });
}
