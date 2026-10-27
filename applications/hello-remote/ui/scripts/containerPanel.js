import { mountInspectionRefresh } from "./inspectionRefresh.js";

export function mountContainerPanel(host, backend, target, isDisposed) {
  const refresh = host.querySelector("[data-container-refresh]");
  const status = host.querySelector("[data-container-status]");
  const facts = host.querySelector("[data-container-facts]");

  const setFact = (name, value) => {
    host.querySelector(`[data-container-${name}]`).textContent = value;
  };
  const show = (info) => {
    setFact("name", info.name);
    setFact("hostname", info.hostname);
    setFact("os", info.operatingSystem);
    setFact("kernel", info.kernel);
    setFact("architecture", info.architecture);
    setFact("cpus", String(info.cpuCount));
    setFact("memory", formatBytes(info.memoryTotalBytes));
    setFact("uptime", formatDuration(info.uptimeSeconds));
    status.hidden = true;
    facts.hidden = false;
  };
  return mountInspectionRefresh({
    refresh,
    isDisposed,
    load: () => backend.call("container", target),
    onLoading: () => {
      status.hidden = false;
      status.textContent = "Inspecting the container…";
    },
    onSuccess: show,
    onFailure: (error) => {
      facts.hidden = true;
      status.textContent = `Container inspection failed: ${error.message}`;
    },
  });
}

export function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes < 0) return "Unknown";
  const gibibytes = bytes / 1024 ** 3;
  return `${gibibytes.toFixed(gibibytes >= 10 ? 0 : 1)} GiB`;
}

export function formatDuration(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return "Unknown";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  if (days > 0) return `${days}d ${hours}h`;
  const minutes = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${minutes}m`;
}
