// Owns the upload observation and one-shot claim state demonstrated by the
// frontend API explorer. Keeping these transitions together prevents callers
// from constructing combinations that the event flow itself cannot produce.
export function createUploadTracker() {
  let count = 0;
  let latest = null;
  let claimNext = false;
  let claimed = 0;

  return {
    armPassThroughClaim() {
      claimNext = true;
    },

    // event.claim must be called synchronously inside the event handler. When
    // armed, the tracker consumes that one moment and deliberately preserves
    // the original path rather than moving or deleting the attachment.
    observe(upload) {
      count += 1;
      latest = upload;
      if (!claimNext) return false;

      claimNext = false;
      upload.claim(Promise.resolve(upload.path).then((path) => {
        claimed += 1;
        return path;
      }));
      return true;
    },

    snapshot() {
      return { count, latest, claimNext, claimed };
    },
  };
}
