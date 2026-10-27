import assert from "node:assert/strict";
import test from "node:test";
import { viewportHeightService } from "./viewportHeightService.ts";

// iPhone 16 Pro Max: a 956pt layout viewport with a 62pt top inset. iOS reports
// the visual viewport 62pt short of it even with the keyboard down.
const IOS_LAYOUT_HEIGHT = 956;
const IOS_VISUAL_HEIGHT = 894;

test("the top inset alone never shortens the app shell", () => {
  // The shell used to be pinned to this shortened height, which left the inset
  // as a strip of bare page background under the composer.
  assert.equal(
    viewportHeightService.keyboardOverride({
      layoutHeight: IOS_LAYOUT_HEIGHT,
      visual: { height: IOS_VISUAL_HEIGHT, offsetTop: 0 },
      inputFocused: true,
    }),
    null
  );

  // Same shortfall without focus, and with no visual viewport at all.
  assert.equal(
    viewportHeightService.keyboardOverride({
      layoutHeight: IOS_LAYOUT_HEIGHT,
      visual: { height: IOS_VISUAL_HEIGHT, offsetTop: 0 },
      inputFocused: false,
    }),
    null
  );
  assert.equal(
    viewportHeightService.keyboardOverride({
      layoutHeight: IOS_LAYOUT_HEIGHT,
      visual: null,
      inputFocused: true,
    }),
    null
  );
});

test("an open keyboard still resizes the app shell", () => {
  assert.deepEqual(
    viewportHeightService.keyboardOverride({
      layoutHeight: IOS_LAYOUT_HEIGHT,
      visual: { height: 620.4, offsetTop: 12.6 },
      inputFocused: true,
    }),
    { height: 620, offsetTop: 13 }
  );

  // A keyboard the user never focused an input for is not a keyboard.
  assert.equal(
    viewportHeightService.keyboardOverride({
      layoutHeight: IOS_LAYOUT_HEIGHT,
      visual: { height: 620, offsetTop: 0 },
      inputFocused: false,
    }),
    null
  );

  // Android's `resizes-content` shrinks the layout viewport too, so `100dvh`
  // has already followed the keyboard and there is nothing left to override.
  assert.equal(
    viewportHeightService.keyboardOverride({
      layoutHeight: 620,
      visual: { height: 620, offsetTop: 0 },
      inputFocused: true,
    }),
    null
  );
});

test("keyboard coverage must exceed the minimum threshold", () => {
  assert.equal(
    viewportHeightService.keyboardOverride({
      layoutHeight: IOS_LAYOUT_HEIGHT,
      visual: { height: IOS_LAYOUT_HEIGHT - 120, offsetTop: 0 },
      inputFocused: true,
    }),
    null
  );

  assert.deepEqual(
    viewportHeightService.keyboardOverride({
      layoutHeight: IOS_LAYOUT_HEIGHT,
      visual: { height: IOS_LAYOUT_HEIGHT - 121, offsetTop: 0 },
      inputFocused: true,
    }),
    { height: IOS_LAYOUT_HEIGHT - 121, offsetTop: 0 }
  );
});
