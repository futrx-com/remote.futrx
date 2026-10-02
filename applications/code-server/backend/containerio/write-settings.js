
const fs = require("fs");
const path = require("path");
const os = require("os");
const active = "/root/.local/share/code-server/User/settings.json";
const input = JSON.parse(fs.readFileSync(0, "utf8"));
if (!input || typeof input !== "object" || Array.isArray(input)) throw new Error("settings must be an object");
if (input["window.title"] === "${rootPath}") input["window.title"] = os.hostname();
const content = JSON.stringify(input, null, 2) + "\n";
// User is linked to persistent storage by the installer. Atomic replacement
// inside that directory preserves the link and is immediately visible to VS Code.
fs.mkdirSync(path.dirname(active), { recursive: true, mode: 0o700 });
const temporary = active + ".remote-tmp";
fs.writeFileSync(temporary, content, { mode: 0o600 });
fs.renameSync(temporary, active);
