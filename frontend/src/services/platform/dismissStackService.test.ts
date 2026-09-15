import assert from "node:assert/strict";
import test from "node:test";
import {
  NO_DISMISS_CLAIM,
  dismissStackService,
} from "./dismissStackService.ts";

/**
 * The service is a single instance, so each test releases what it took. A
 * helper returns the releases in reverse, the order a component tree unmounts.
 */
function open(...options: Array<{ fallback?: boolean } | undefined>): {
  claims: number[];
  releaseAll: () => void;
} {
  const claims = options.map((option) => dismissStackService.claim(option));
  return {
    claims,
    releaseAll: () => [...claims].reverse().forEach((claim) => dismissStackService.release(claim)),
  };
}

test("the newest surface open owns the dismissal", () => {
  const { claims, releaseAll } = open(undefined, undefined, undefined);
  const [first, second, third] = claims;

  assert.equal(dismissStackService.owns(third), true);
  assert.equal(dismissStackService.owns(second), false);
  assert.equal(dismissStackService.owns(first), false);

  releaseAll();
});

test("closing the frontmost surface hands the dismissal back to the one behind", () => {
  const { claims, releaseAll } = open(undefined, undefined);
  const [behind, front] = claims;

  dismissStackService.release(front);
  assert.equal(dismissStackService.owns(behind), true);

  releaseAll();
});

test("a surface opened later gets in front of one already listening", () => {
  // The case listener order cannot express: find-in-chat is up, then a modal
  // opens over it. The modal must take the next Escape.
  const { claims, releaseAll } = open(undefined);
  const [find] = claims;
  assert.equal(dismissStackService.owns(find), true);

  const modal = dismissStackService.claim();
  assert.equal(dismissStackService.owns(modal), true);
  assert.equal(dismissStackService.owns(find), false);

  // ...and closing it gives find-in-chat the key back.
  dismissStackService.release(modal);
  assert.equal(dismissStackService.owns(find), true);

  releaseAll();
});

test("a surface closed out of order leaves the rest in their order", () => {
  const { claims, releaseAll } = open(undefined, undefined, undefined);
  const [first, middle, last] = claims;

  dismissStackService.release(middle);
  assert.equal(dismissStackService.owns(last), true);

  dismissStackService.release(last);
  assert.equal(dismissStackService.owns(first), true);

  releaseAll();
});

test("a fallback waits behind every surface, however long it has been open", () => {
  // Escape cancels a streaming reply only when nothing is open over it, and
  // starting the run does not put it in front of a bar that was already up.
  const { claims, releaseAll } = open(undefined);
  const [find] = claims;
  const streaming = dismissStackService.claim({ fallback: true });

  assert.equal(dismissStackService.owns(find), true);
  assert.equal(dismissStackService.owns(streaming), false);

  dismissStackService.release(find);
  assert.equal(dismissStackService.owns(streaming), true);

  releaseAll();
  dismissStackService.release(streaming);
});

test("a fallback owns the dismissal while it is the only claim", () => {
  const streaming = dismissStackService.claim({ fallback: true });
  assert.equal(dismissStackService.owns(streaming), true);
  dismissStackService.release(streaming);
});

test("the newest fallback owns it when no surface is open", () => {
  const older = dismissStackService.claim({ fallback: true });
  const newer = dismissStackService.claim({ fallback: true });

  assert.equal(dismissStackService.owns(newer), true);
  assert.equal(dismissStackService.owns(older), false);

  dismissStackService.release(newer);
  dismissStackService.release(older);
});

test("nothing owns a dismissal once every claim is released", () => {
  const { claims, releaseAll } = open(undefined, { fallback: true });
  releaseAll();

  for (const claim of claims) assert.equal(dismissStackService.owns(claim), false);
});

test("a claim never taken owns nothing", () => {
  assert.equal(dismissStackService.owns(NO_DISMISS_CLAIM), false);

  // A surface is open, so there is something to be wrong about.
  const { releaseAll } = open(undefined);
  assert.equal(dismissStackService.owns(NO_DISMISS_CLAIM), false);
  releaseAll();
});

test("releasing twice does not resurrect the claim behind it", () => {
  const { claims, releaseAll } = open(undefined, undefined);
  const [behind, front] = claims;

  dismissStackService.release(front);
  dismissStackService.release(front);
  assert.equal(dismissStackService.owns(behind), true);
  assert.equal(dismissStackService.owns(front), false);

  releaseAll();
});

test("claims stay distinct after a surface reopens", () => {
  // A remounted modal must not answer to the id its previous mount held.
  const first = dismissStackService.claim();
  dismissStackService.release(first);
  const second = dismissStackService.claim();

  assert.notEqual(second, first);
  assert.equal(dismissStackService.owns(first), false);
  assert.equal(dismissStackService.owns(second), true);

  dismissStackService.release(second);
});
