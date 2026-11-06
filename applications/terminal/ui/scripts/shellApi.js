// Terminal's backend routes on the drawer's project installation.
export function createShellApi(remote, projectId) {
  async function request(path, options) {
    const response = await remote.backend.fetch(path, { ...options, projectId });
    if (!response.ok) {
      const error = new Error(`terminal backend answered ${response.status}`);
      error.status = response.status;
      throw error;
    }
    return response.json();
  }

  return {
    attach: ({ session, cwd, cols, rows }) =>
      request("sessions", { method: "POST", body: { session, cwd, cols, rows } }),
    read: ({ session, generation, offset }, signal) =>
      request("output", { method: "GET", query: { session, generation, offset }, signal }),
    write: ({ session, generation }, data) =>
      request("input", { method: "POST", query: { session, generation }, body: data }),
    resize: ({ session, generation, cols, rows }) =>
      request("resize", { method: "POST", body: { session, generation, cols, rows } }),
  };
}

/** Output bytes arrive base64-encoded inside the JSON response. */
export function decodeOutput(data) {
  if (!data) return new Uint8Array(0);
  const binary = atob(data);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index);
  return bytes;
}
