// Moves finished chat attachments onto the mounted bucket.
//
// Remote writes an attachment from the host, into the directory the container
// sees as /workspace, so it lands in /workspace/.uploads — beside the mount,
// never inside it. A host-side write could not reach the mount even if it
// aimed at one: FUSE exists in the container's mount namespace. So the move
// has to happen in the container, which is what this image's plugin does over
// `POST push`. This module is the trigger.
//
// It claims each attachment, so the composer waits: the prompt names the file
// it is handing the agent, and that name has to be the file's final home. Once
// the plugin reports the attachment removed from .uploads, the claim resolves
// with its path on the mount and the prompt points there instead.
//
// Nothing here is authoritative. The event fires in the browser, so an upload
// written by an agent or the CLI is not covered. `recent()` exists so the
// mount controls can show what actually happened.

const RECENT_LIMIT = 20;

export function createUploadSync(remote) {
  const recent = [];

  function record(entry) {
    recent.unshift({ at: Date.now(), ...entry });
    recent.length = Math.min(recent.length, RECENT_LIMIT);
  }

  // s3disk installs per project, so an attachment is only ever moved to the
  // bucket belonging to its own project. Falling back to "any running
  // instance" would put one project's uploads in another's bucket, which is
  // why this resolves explicitly instead of passing projectId as a preference
  // to remote.backend.
  function instanceFor(projectId) {
    if (!projectId) return null;
    return remote.backend.instances.find(
      (candidate) => candidate.projectId === projectId,
    ) ?? null;
  }

  async function push(event, instance) {
    let outcome;
    try {
      outcome = await remote.backend.call("push", {
        method: "POST",
        instanceId: instance.instanceId,
        body: { names: [event.fileName] },
      });
    } catch (error) {
      record({ name: event.fileName, error: error.message });
      remote.log(`${event.fileName}: ${error.message}`);
      return undefined;
    }

    const result = outcome.results?.[0] ?? {};
    record({
      name: event.fileName,
      path: result.path,
      stored: Boolean(result.stored),
      removed: Boolean(result.removed),
      skipped: Boolean(result.skipped),
      error: result.error,
    });
    if (result.error) remote.log(`${event.fileName}: ${result.error}`);

    // The prompt is only re-pointed once .uploads no longer holds the file.
    // While both copies exist the path the composer already has still
    // resolves, and leaving it alone is the outcome that cannot break a send.
    return result.removed ? result.path : undefined;
  }

  return {
    /** Subscribes to upload completions. Returns an unsubscribe. */
    start() {
      // A Remote too old to emit upload.completed has no `events` at all, and
      // reaching through it would throw out of activate() — taking the mount
      // controls with it. Nothing about the mount needs this, so say what is
      // missing and let the rest of the extension load.
      if (typeof remote.events?.on !== "function") {
        remote.log(
          "This Remote does not emit upload.completed; chat attachments will stay in .uploads.",
        );
        return () => {};
      }
      return remote.events.on("upload.completed", (event) => {
        const instance = instanceFor(event.projectId);
        // Not this project's upload: do not claim it, so nothing waits on us.
        if (!instance) return;
        event.claim(push(event, instance));
      });
    },
    /** Most recent moves first, for the mount controls to display. */
    recent: () => [...recent],
  };
}

export function describeRecent(entries) {
  if (!entries.length) return "No attachments moved in this session.";
  return entries
    .map((entry) => {
      const time = new Date(entry.at).toLocaleTimeString();
      if (entry.error) {
        const state = entry.stored ? "on the bucket, still in .uploads" : "failed";
        return `${time}  ${entry.name} — ${state}: ${entry.error}`;
      }
      const stored = entry.skipped ? "already on the bucket" : "moved to";
      return `${time}  ${entry.name} — ${stored} ${entry.path}`;
    })
    .join("\n");
}
