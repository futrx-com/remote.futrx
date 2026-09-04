import assert from "node:assert/strict";
import test from "node:test";
import { returnUrlPolicy } from "./returnUrlPolicy.ts";

const configuredOrigin = "https://remote.example.com";

test("accepts the configured origin and its project subdomains", () => {
  const originTarget = "https://remote.example.com/settings";
  const projectTarget = "https://project--3000.dev.remote.example.com/chat";

  assert.equal(returnUrlPolicy.safeTarget(originTarget, configuredOrigin), originTarget);
  assert.equal(returnUrlPolicy.safeTarget(projectTarget, configuredOrigin), projectTarget);
});

test("rejects external and deceptive return URL origins", () => {
  assert.equal(returnUrlPolicy.safeTarget("https://attacker.example/phish", configuredOrigin), "");
  assert.equal(
    returnUrlPolicy.safeTarget(
      "https://remote.example.com.attacker.example/phish",
      configuredOrigin
    ),
    ""
  );
});
