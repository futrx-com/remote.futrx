const fs = require("fs");
const path = require("path");
const root = "/workspace/.remote/code-server";
const durable = path.join(root, "User");
const active = "/root/.local/share/code-server/User";
const legacy = path.join(root, "settings.json");
fs.mkdirSync(durable, { recursive: true, mode: 0o700 });
fs.chmodSync(root, 0o700);
fs.chmodSync(durable, 0o700);
fs.mkdirSync(path.dirname(active), { recursive: true });
const current = fs.lstatSync(active, { throwIfNoEntry: false });
const linked = current?.isSymbolicLink() && fs.realpathSync(active) === fs.realpathSync(durable);
if (current && !linked) {
  // Existing editor changes win over the old saved copy. Copy bytes unchanged,
  // including JSON with comments, keybindings, profiles and extension state.
  fs.cpSync(fs.realpathSync(active), durable, { recursive: true, force: true });
}
const settingsFile = path.join(durable, "settings.json");
if (!fs.existsSync(settingsFile)) {
  if (fs.existsSync(legacy)) {
    fs.copyFileSync(legacy, settingsFile);
  } else {
    const settings = JSON.parse(process.env.CODE_SERVER_SETTINGS_JSON || "null");
    if (!settings || typeof settings !== "object" || Array.isArray(settings)) {
      throw new Error("Code Server settings must be a JSON object");
    }
    if (settings["window.title"] === "${rootPath}") {
      settings["window.title"] = process.env.CODE_SERVER_WS_NAME;
    }
    fs.writeFileSync(settingsFile, JSON.stringify(settings, null, 2) + "\n", { mode: 0o600 });
  }
}
fs.chmodSync(settingsFile, 0o600);
if (!linked) {
  // Keep the original until the link is installed successfully.
  const backup = fs.mkdtempSync(path.join(path.dirname(active), ".User-migration-"));
  const saved = path.join(backup, "User");
  if (current) fs.renameSync(active, saved);
  try {
    fs.symlinkSync(durable, active, "dir");
  } catch (error) {
    if (current) fs.renameSync(saved, active);
    throw error;
  }
  fs.rmSync(backup, { recursive: true, force: true });
}
// The old independent copy is no longer authoritative.
fs.rmSync(legacy, { force: true });
