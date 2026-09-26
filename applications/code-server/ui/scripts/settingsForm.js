export function validateSettingsText(value) {
  if (new TextEncoder().encode(value).length > 128 * 1024) {
    throw new Error("Settings must be no larger than 128 KiB.");
  }
  let settings;
  try {
    settings = JSON.parse(value);
  } catch (error) {
    throw new Error(`Invalid JSON: ${error.message}`);
  }
  if (settings === null || typeof settings !== "object" || Array.isArray(settings)) {
    throw new Error("Settings must be a JSON object.");
  }
  return settings;
}

export function openSettingsForm(remote, instanceId) {
  remote.ui.openPopup({
    title: "Code Server settings",
    width: 760,
    mount: (body) => {
      const form = document.createElement("form");
      form.className = "code-server-settings";

      const label = document.createElement("label");
      label.textContent = "VS Code settings.json";
      const editor = document.createElement("textarea");
      editor.id = `code-server-settings-${instanceId}`;
      editor.spellcheck = false;
      editor.disabled = true;
      label.htmlFor = editor.id;

      const help = document.createElement("p");
      help.textContent = "These settings apply to this project's Code Server. Some changes may need an editor reload.";
      const status = document.createElement("p");
      status.setAttribute("role", "status");
      status.textContent = "Loading settings…";

      const actions = document.createElement("div");
      actions.className = "code-server-settings__actions";
      const save = document.createElement("button");
      save.type = "submit";
      save.textContent = "Save settings";
      save.disabled = true;
      actions.append(save);
      form.append(label, editor, help, status, actions);
      body.replaceChildren(form);

      let closed = false;
      const target = { instanceId };
      void remote.backend.call("settings", target).then((reply) => {
        if (closed) return;
        editor.value = reply.settings;
        editor.disabled = false;
        save.disabled = false;
        status.textContent = "";
      }).catch((error) => {
        if (!closed) status.textContent = `Could not load settings: ${error.message}`;
      });

      const onSubmit = async (event) => {
        event.preventDefault();
        try {
          validateSettingsText(editor.value);
        } catch (error) {
          status.textContent = error.message;
          return;
        }
        save.disabled = true;
        status.textContent = "Saving…";
        try {
          const reply = await remote.backend.call("settings", {
            ...target, method: "POST", body: { settings: editor.value },
          });
          if (closed) return;
          editor.value = reply.settings;
          status.textContent = "Settings saved.";
        } catch (error) {
          if (!closed) status.textContent = `Could not save settings: ${error.message}`;
        } finally {
          if (!closed) save.disabled = false;
        }
      };
      form.addEventListener("submit", onSubmit);
      return () => { closed = true; form.removeEventListener("submit", onSubmit); };
    },
  });
}
