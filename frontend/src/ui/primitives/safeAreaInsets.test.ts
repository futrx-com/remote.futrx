import { strict as assert } from "node:assert";
import { describe, it } from "node:test";

import { SAFE_AREA_EDGES, safeAreaInset, safeAreaStyle } from "./safeAreaInsets.ts";

describe("safeAreaInset", () => {
  it("resolves each edge to its env() inset with a zero fallback", () => {
    assert.equal(safeAreaInset("top"), "env(safe-area-inset-top, 0px)");
    assert.equal(safeAreaInset("right"), "env(safe-area-inset-right, 0px)");
    assert.equal(safeAreaInset("bottom"), "env(safe-area-inset-bottom, 0px)");
    assert.equal(safeAreaInset("left"), "env(safe-area-inset-left, 0px)");
  });
});

describe("safeAreaStyle", () => {
  it("pads every edge by default", () => {
    assert.deepEqual(safeAreaStyle(), {
      paddingTop: "env(safe-area-inset-top, 0px)",
      paddingRight: "env(safe-area-inset-right, 0px)",
      paddingBottom: "env(safe-area-inset-bottom, 0px)",
      paddingLeft: "env(safe-area-inset-left, 0px)",
    });
    assert.deepEqual([...SAFE_AREA_EDGES], ["top", "right", "bottom", "left"]);
  });

  it("pads only the requested edges", () => {
    assert.deepEqual(safeAreaStyle(["top"]), {
      paddingTop: "env(safe-area-inset-top, 0px)",
    });
    assert.deepEqual(safeAreaStyle(["bottom", "left"]), {
      paddingBottom: "env(safe-area-inset-bottom, 0px)",
      paddingLeft: "env(safe-area-inset-left, 0px)",
    });
  });

  it("applies a repeated edge once and no edges as no padding", () => {
    assert.deepEqual(safeAreaStyle(["top", "top"]), {
      paddingTop: "env(safe-area-inset-top, 0px)",
    });
    assert.deepEqual(safeAreaStyle([]), {});
  });
});
