// Presentation and interaction for the frontend capability explorer. The
// showcase composition module decides where this UI appears; this module owns
// only its DOM lifecycle and the extension API calls it demonstrates.

export function mountSettingsShowcase(host, remote, context, uploads) {
  const panel = document.createElement("section");
  panel.className = "hello-remote hello-remote__showcase-card";

  const copy = document.createElement("div");
  const title = document.createElement("h4");
  title.className = "hello-remote__title";
  title.textContent = "Hello Remote frontend API";
  const note = document.createElement("p");
  note.className = "hello-remote__note";
  note.textContent = "This panel was mounted through ui.register in the project settings slot.";
  copy.append(title, note);

  const button = document.createElement("button");
  button.type = "button";
  button.className = "hello-remote__button";
  button.textContent = "Explore API";
  const open = () => openFrontendExplorer(remote, context, uploads);
  button.addEventListener("click", open);

  panel.append(copy, button);
  host.appendChild(panel);
  return () => button.removeEventListener("click", open);
}

export function openFrontendExplorer(remote, context, uploads) {
  let popup;
  popup = remote.ui.openPopup({
    title: "Hello Remote · Frontend API",
    width: 720,
    html: explorerMarkup(),
    mount: (body) => mountExplorer(body, remote, context, uploads, () => popup.close()),
  });
  // The returned handle exposes the live body as well as close(). Marking it
  // here makes that half of the popup API visible in browser inspection.
  popup.body.dataset.helloRemoteExplorer = "true";
}

function mountExplorer(body, remote, context, uploads, close) {
  const output = body.querySelector("[data-showcase-output]");
  const controls = new AbortController();
  const target = { projectId: context.projectId };

  showExplorerFacts(body, remote, context, uploads.snapshot());

  const logo = body.querySelector("[data-showcase-logo]");
  logo.src = remote.assets.url("assets/logo.svg");

  for (const button of body.querySelectorAll("[data-showcase-action]")) {
    button.addEventListener("click", () => {
      invokeExplorerAction(
        button,
        remote,
        context,
        target,
        output,
        uploads,
        controls.signal,
        close
      );
    }, { signal: controls.signal });
  }

  return () => controls.abort();
}

function showExplorerFacts(body, remote, context, uploadSnapshot) {
  setText(body, "api-version", String(remote.apiVersion));
  setText(body, "application", `${remote.application.name} ${remote.application.version ?? ""}`.trim());
  setText(body, "application-id", remote.application.id);
  setText(body, "install", installSummary(remote.install));
  setText(body, "slot", context.slot);
  setText(body, "context", contextSummary(context));
  setText(body, "backend", backendSummary(remote.backend));
  setText(body, "uploads", uploadSummary(uploadSnapshot));
}

function invokeExplorerAction(button, remote, context, target, output, uploads, signal, close) {
  switch (button.dataset.showcaseAction) {
    case "call":
      void showResult(button, output, () => remote.backend.call("echo", {
        ...target,
        method: "POST",
        body: { message: "Hello from the frontend API" },
        query: { source: "frontend-showcase" },
        headers: { "X-Hello-Remote": "frontend-showcase" },
        signal,
      }));
      break;
    case "fetch":
      void showResult(button, output, async () => {
        const response = await remote.backend.fetch("hello", {
          ...target,
          headers: { "X-Hello-Remote": "frontend-showcase" },
          signal,
        });
        return {
          status: response.status,
          contentType: response.headers.get("Content-Type"),
          body: await response.json(),
        };
      });
      break;
    case "describe":
      void showResult(button, output, () => remote.backend.describe(target));
      break;
    case "url":
      void showResult(button, output, () => ({
        backend: remote.backend.url("hello", target),
        view: remote.views.url("panel"),
        asset: remote.assets.url("assets/logo.svg"),
      }));
      break;
    case "view":
      void showResult(button, output, async () => {
        const html = await remote.views.load("panel");
        return { name: "panel", characters: html.length, preview: html.slice(0, 180) };
      });
      break;
    case "log":
      remote.log("frontend API explorer", { context, install: remote.install });
      output.textContent = "Context written through remote.log. Open the browser console to inspect it.";
      break;
    case "claim":
      uploads.armPassThroughClaim();
      output.textContent =
        "The next upload in this tab will use event.claim() and resolve to its original path.";
      break;
    case "close":
      close();
      break;
  }
}

async function showResult(button, output, work) {
  button.disabled = true;
  output.textContent = "Working…";
  try {
    const result = await work();
    output.textContent = typeof result === "string"
      ? result
      : JSON.stringify(result, null, 2);
  } catch (error) {
    output.textContent = `Error: ${error.message}`;
  } finally {
    button.disabled = false;
  }
}

function explorerMarkup() {
  return `
    <div class="hello-remote__explorer">
      <div class="hello-remote__explorer-heading">
        <img data-showcase-logo alt="" />
        <div>
          <h3>Extension capability explorer</h3>
          <p>Opened by a contribution in <code data-showcase-slot></code>.</p>
        </div>
      </div>
      <dl class="hello-remote__api-facts">
        <div><dt>API version</dt><dd data-showcase-api-version></dd></div>
        <div><dt>Application</dt><dd data-showcase-application></dd></div>
        <div><dt>Application ID</dt><dd data-showcase-application-id></dd></div>
        <div><dt>Installed in</dt><dd data-showcase-install></dd></div>
        <div><dt>Slot context</dt><dd data-showcase-context></dd></div>
        <div><dt>Backend</dt><dd data-showcase-backend></dd></div>
        <div><dt>Upload events seen</dt><dd data-showcase-uploads></dd></div>
      </dl>
      <div class="hello-remote__api-actions">
        <button class="hello-remote__button" type="button" data-showcase-action="call">backend.call</button>
        <button class="hello-remote__button" type="button" data-showcase-action="fetch">backend.fetch</button>
        <button class="hello-remote__button" type="button" data-showcase-action="describe">backend.describe</button>
        <button class="hello-remote__button" type="button" data-showcase-action="url">URL helpers</button>
        <button class="hello-remote__button" type="button" data-showcase-action="view">views.load</button>
        <button class="hello-remote__button" type="button" data-showcase-action="log">remote.log</button>
        <button class="hello-remote__button" type="button" data-showcase-action="claim">claim next upload</button>
        <button class="hello-remote__button" type="button" data-showcase-action="close">popup.close</button>
      </div>
      <pre class="hello-remote__api-output" data-showcase-output>Select an ability to see its result.</pre>
    </div>`;
}

function setText(root, name, value) {
  root.querySelector(`[data-showcase-${name}]`).textContent = value;
}

export function installSummary(install) {
  const locations = [];
  if (install.global) locations.push("global");
  locations.push(...install.projectIds.map((id) => `project ${id}`));
  return locations.join(", ") || "nowhere";
}

export function contextSummary(context) {
  const values = [
    ["scope", context.scope],
    ["project", context.projectName ?? context.projectId],
    ["chat", context.chatId],
    ["cwd", context.cwd],
    ["instance", context.instance?.id],
  ].filter(([, value]) => value);
  return values.length ? values.map(([name, value]) => `${name}: ${value}`).join(" · ") : "no optional fields";
}

export function backendSummary(backend) {
  return backend.available
    ? `${backend.instances.length} running instance${backend.instances.length === 1 ? "" : "s"}`
    : "unavailable";
}

export function uploadSummary(observedUploads) {
  const claim = observedUploads.claimNext
    ? " · pass-through claim armed"
    : observedUploads.claimed
      ? ` · ${observedUploads.claimed} pass-through claim${observedUploads.claimed === 1 ? "" : "s"}`
      : "";
  if (!observedUploads.latest) {
    return `${observedUploads.count} · upload.completed is being observed${claim}`;
  }
  return `${observedUploads.count} · latest: ${observedUploads.latest.fileName} (${observedUploads.latest.size} bytes)${claim}`;
}
