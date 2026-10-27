import { mountInspectionRefresh } from "./inspectionRefresh.js";

export function mountServicePanel(host, backend, target, isDisposed) {
  const refresh = host.querySelector("[data-service-refresh]");
  const status = host.querySelector("[data-service-status]");
  const facts = host.querySelector("[data-service-facts]");

  const setFact = (name, value) => {
    host.querySelector(`[data-service-${name}]`).textContent = value;
  };
  const show = (info) => {
    setFact("unit", info.service);
    setFact("version", info.version);
    setFact("provisioning", info.provisionedVersion);
    setFact("ports", portSummary(info.externalPort, info.internalPort));
    status.textContent = info.message;
    facts.hidden = false;
  };
  return mountInspectionRefresh({
    refresh,
    isDisposed,
    load: () => backend.call("service", target),
    onLoading: () => {
      status.textContent = "Calling the supervised service…";
    },
    onSuccess: show,
    onFailure: (error) => {
      facts.hidden = true;
      status.textContent = `Service inspection failed: ${error.message}`;
    },
  });
}

export function portSummary(externalPort, internalPort) {
  if (!Number.isInteger(externalPort) || !Number.isInteger(internalPort)) {
    return "Unknown";
  }
  return `127.0.0.1:${externalPort} → container:${internalPort}/tcp`;
}
