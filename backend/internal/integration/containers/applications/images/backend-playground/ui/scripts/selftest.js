// Asserts the backend plugin contract from inside a real extension, against a
// real plugin process, over the real HTTP route. It is the fastest way to know
// whether a change to the contract, the host, or the transport broke anything:
// install this image, open Applications, click one button.
//
// Each check returns { name, ok, detail }. A check that throws is a failure
// with the thrown message as its detail, so a broken host reports as a red
// line rather than an empty popup.

export async function runBackendSelfTest(remote, context) {
  const target = { projectId: context?.projectId };
  const call = (path, options) =>
    remote.backend.call(path, { ...options, projectId: context?.projectId });

  const checks = [
    {
      name: "the extension has a running backend to call",
      run: () => {
        if (!remote.backend.available) throw new Error("remote.backend.available is false");
        return `${remote.backend.instances.length} instance(s)`;
      },
    },
    {
      name: "describe reports this contract version and its routes",
      run: async () => {
        const described = await remote.backend.describe(target);
        const version = described.descriptor.apiVersion;
        if (version !== 1) throw new Error(`apiVersion ${version}, expected 1`);
        const routes = described.descriptor.routes ?? [];
        if (routes.length < 10) throw new Error(`only ${routes.length} routes advertised`);
        return `${routes.length} routes, access ${described.access}`;
      },
    },
    {
      name: "health answers from a live process",
      run: async () => {
        const health = await call("health");
        if (!health.ok || !health.pid) throw new Error(JSON.stringify(health));
        return `pid ${health.pid}, ${health.goVersion}`;
      },
    },
    {
      name: "the same process serves consecutive calls",
      run: async () => {
        const first = await call("health");
        const second = await call("health");
        if (first.pid !== second.pid) {
          throw new Error(`pid changed ${first.pid} → ${second.pid}`);
        }
        if (second.requests <= first.requests) {
          throw new Error("the request counter did not advance");
        }
        return `pid ${second.pid} held state across calls`;
      },
    },
    {
      name: "the plugin receives method, query, and body",
      run: async () => {
        const echoed = await call("echo", {
          method: "POST",
          query: { probe: "self-test" },
          body: { value: 42 },
        });
        if (echoed.method !== "POST") throw new Error(`method ${echoed.method}`);
        if (echoed.query?.probe?.[0] !== "self-test") throw new Error("query was not forwarded");
        if (!echoed.body.includes("42")) throw new Error("body was not forwarded");
        return "method, query, and body arrived intact";
      },
    },
    {
      name: "the caller is stamped by the server",
      run: async () => {
        const echoed = await call("echo");
        if (!echoed.caller?.email) throw new Error("no caller email");
        return `${echoed.caller.email}${echoed.caller.isAdmin ? " (admin)" : ""}`;
      },
    },
    {
      name: "the session cookie is withheld from the plugin",
      run: async () => {
        const echoed = await call("echo");
        const names = Object.keys(echoed.headers ?? {}).map((name) => name.toLowerCase());
        const leaked = names.filter((name) => name === "cookie" || name === "authorization");
        if (leaked.length) throw new Error(`forwarded ${leaked.join(", ")}`);
        return `${names.length} headers forwarded, no credentials`;
      },
    },
    {
      name: "state written in one call is readable in the next",
      run: async () => {
        const value = `self-test ${Date.now()}`;
        await call("kv/selftest", { method: "POST", body: { value } });
        const read = await call("kv/selftest");
        if (read.value !== value) throw new Error(`read back ${read.value}`);
        return "in-memory state survives between requests";
      },
    },
    {
      name: "the plugin's data directory is writable",
      run: async () => {
        const note = `self-test ${Date.now()}`;
        const written = await call("notes", { method: "POST", body: { note } });
        const read = await call("notes");
        if (read.note !== note) throw new Error(`read back ${read.note}`);
        return written.path;
      },
    },
    {
      name: "real Go work runs on the server",
      run: async () => {
        const computed = await call("compute", { method: "POST", body: { n: 30 } });
        if (computed.fibonacci !== 832040) throw new Error(`fib(30) = ${computed.fibonacci}`);
        return `fib(30) and ${computed.primes} primes in ${Math.round(computed.elapsedNs / 1000)}µs`;
      },
    },
    {
      name: "the instance the host handed over is this image",
      run: async () => {
        const instance = await call("instance");
        if (instance.imageId !== remote.image.id) {
          throw new Error(`imageId ${instance.imageId}`);
        }
        if (!instance.dataDir) throw new Error("no data directory assigned");
        return `${instance.scope}${instance.projectId ? ` · ${instance.projectId}` : ""}`;
      },
    },
    {
      name: "an unknown route is a 404, not a hang",
      run: async () => {
        await expectFailure(call("no/such/route"), "no route");
        return "unknown routes are refused by the plugin's mux";
      },
    },
    {
      name: "a wrong method is refused",
      run: async () => {
        await expectFailure(call("health", { method: "POST", body: {} }), "not allowed");
        return "405 rather than a silent GET";
      },
    },
    {
      name: "a panicking route costs one request, not the process",
      run: async () => {
        const before = await call("health");
        await expectFailure(call("boom"), "panic");
        const after = await call("health");
        if (after.pid !== before.pid) {
          throw new Error(`the plugin restarted: ${before.pid} → ${after.pid}`);
        }
        return `pid ${after.pid} survived the panic`;
      },
    },
  ];

  const results = [];
  for (const check of checks) {
    try {
      const detail = await check.run();
      results.push({ name: check.name, ok: true, detail: detail ?? "" });
    } catch (error) {
      results.push({ name: check.name, ok: false, detail: String(error?.message ?? error) });
    }
  }
  return results;
}

// expectFailure asserts that a call rejects, and that it rejects for the
// reason expected — a check that passes because everything failed would be
// worse than no check.
async function expectFailure(promise, expected) {
  let resolved;
  try {
    resolved = await promise;
  } catch (error) {
    const message = String(error?.message ?? error).toLowerCase();
    if (!message.includes(expected.toLowerCase())) {
      throw new Error(`failed, but with: ${message}`);
    }
    return;
  }
  throw new Error(`expected a failure, got ${JSON.stringify(resolved)}`);
}
