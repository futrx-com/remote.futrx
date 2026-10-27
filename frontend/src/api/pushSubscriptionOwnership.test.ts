import assert from "node:assert/strict";
import test from "node:test";

import {
  reconcileSubscriptionOwnership,
  revokeSubscriptionForLogout,
} from "./pushSubscriptionOwnership.ts";

const subscription = { endpoint: "https://push.example.com/browser-device" };

test("an endpoint owned by the signed-in account remains subscribed", async () => {
  let invalidations = 0;
  const ownership = await reconcileSubscriptionOwnership(
    subscription,
    async () => true,
    async () => {
      invalidations++;
    }
  );

  assert.equal(ownership, "owned");
  assert.equal(invalidations, 0);
});

test("an endpoint belonging to another account is invalidated locally", async () => {
  let invalidations = 0;
  const ownership = await reconcileSubscriptionOwnership(
    subscription,
    async () => false,
    async () => {
      invalidations++;
    }
  );

  assert.equal(ownership, "foreign");
  assert.equal(invalidations, 1);
});

test("an unreachable server leaves the registration alone", async () => {
  let invalidations = 0;
  const ownership = await reconcileSubscriptionOwnership(
    subscription,
    async () => {
      throw new Error("offline");
    },
    async () => {
      invalidations++;
    }
  );

  assert.equal(ownership, "unverified");
  assert.equal(invalidations, 0);
});

test("logout invalidates the browser even when server cleanup fails", async () => {
  let invalidations = 0;

  await assert.rejects(
    () =>
      revokeSubscriptionForLogout(
        subscription,
        async () => {
          throw new Error("offline");
        },
        async () => {
          invalidations++;
        }
      ),
    /offline/
  );
  assert.equal(invalidations, 1);
});
