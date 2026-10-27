import assert from "node:assert/strict";
import test from "node:test";

import {
  EXTENSION_SLOTS,
  EXTENSION_SLOT_NAMES,
  SLOT_ICON_APPEARANCE,
} from "./extensions.ts";

test("slot names are unique", () => {
  assert.equal(
    new Set(EXTENSION_SLOT_NAMES).size,
    EXTENSION_SLOT_NAMES.length
  );
});

test("every slot declares an icon appearance", () => {
  // addIconButton reads this map by slot; a slot missing an entry would render
  // an unsized button rather than matching its neighbours.
  for (const slot of EXTENSION_SLOT_NAMES) {
    const appearance = SLOT_ICON_APPEARANCE[slot];
    assert.ok(appearance, `no appearance for ${slot}`);
    assert.match(appearance.button, /\bh-\d/, `${slot} button has no height`);
    assert.match(appearance.icon, /\bh-\d/, `${slot} icon has no height`);
  }
});
