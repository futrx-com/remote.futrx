// Connection popup for one installed MySQL instance. Kept separate from the
// entry module to show that an extension is ordinary ES modules: relative
// imports resolve against the image's own ui/ directory.

/**
 * Fills the popup body with credentials for one instance.
 *
 * @param {HTMLElement} body   popup body, already holding views/popup.html
 * @param {object} remote      the extension API
 * @param {object} instance    the installed instance from the slot context
 */
export async function mountConnectionPopup(body, remote, instance) {
  const set = (field, value) => {
    const node = body.querySelector(`[data-field="${field}"]`);
    if (node) node.textContent = value ?? "—";
  };
  const fail = (message) => {
    const node = body.querySelector('[data-field="error"]');
    if (!node) return;
    node.textContent = message;
    node.hidden = false;
  };

  set("host", instance.bindAddress);
  set("port", String(instance.externalPort));

  let credentials;
  try {
    credentials = await fetchCredentials(instance);
  } catch (error) {
    fail(`Could not load credentials: ${error.message}`);
    remote.log("credentials failed", error);
    return;
  }

  set("username", credentials.username);
  set("password", credentials.password);
  set("database", credentials.database);

  const url = connectionUrl(credentials);
  const copy = body.querySelector('[data-action="copy"]');
  copy?.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(url);
      copy.textContent = "Copied";
    } catch {
      // Clipboard access can be denied; showing the URL is the fallback.
      copy.textContent = url;
    }
  });
}

// Project instances are addressed through their project so membership is
// checked; global ones through the server-wide route.
async function fetchCredentials(instance) {
  const path = instance.projectId
    ? `/api/projects/${encodeURIComponent(instance.projectId)}/applications/${encodeURIComponent(instance.id)}/credentials`
    : `/api/applications/${encodeURIComponent(instance.id)}/credentials`;
  const response = await fetch(path, { credentials: "same-origin" });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return response.json();
}

function connectionUrl(credentials) {
  const user = encodeURIComponent(credentials.username ?? "root");
  const password = encodeURIComponent(credentials.password ?? "");
  const database = credentials.database ? `/${credentials.database}` : "";
  return `mysql://${user}:${password}@${credentials.bindAddress}:${credentials.externalPort}${database}`;
}
