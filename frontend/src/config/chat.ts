export function modelShortLabel(model?: string): string {
  return model || "auto";
}

export function providerDisplayLabel(provider?: string): string {
  if (!provider) return "Codex";
  return provider.charAt(0).toUpperCase() + provider.slice(1);
}
