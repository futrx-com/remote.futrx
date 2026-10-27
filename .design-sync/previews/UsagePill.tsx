// UsagePill — token/cost usage indicator from the thread header.
import { UsagePill } from "remote.futrx-web";

const totals = {
  inputTokens: 182_400,
  outputTokens: 24_310,
  cacheReadTokens: 96_000,
  cacheWriteTokens: 12_800,
};

export const WithCost = () => (
  <UsagePill totals={totals} tokenLabel="206.7k" costUsd={1.84} />
);

export const TinyCost = () => (
  <UsagePill totals={totals} tokenLabel="8.2k" costUsd={0.0042} />
);

export const NoCost = () => (
  <UsagePill totals={totals} tokenLabel="12.5k" costUsd={0} />
);
