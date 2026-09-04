// The first-boot setup token reaches the browser as a "?token=" query
// parameter.
class SetupTokenPolicy {
  // read pulls the token out of a location search string. It tolerates the
  // search being absent, empty, or carrying unrelated parameters.
  read(rawSearch: string): string {
    return new URLSearchParams(rawSearch).get("token")?.trim() ?? "";
  }

  // strippedUrl is the address to rewrite to once the token has been read
  // into memory, so it stops sitting in the address bar and the history
  // entry. Unrelated query parameters (e.g. return_to) are preserved.
  strippedUrl(pathname: string, search: string): string {
    const params = new URLSearchParams(search);
    params.delete("token");
    const query = params.toString();
    return `${pathname}${query ? `?${query}` : ""}`;
  }
}

export const setupTokenPolicy = new SetupTokenPolicy();
