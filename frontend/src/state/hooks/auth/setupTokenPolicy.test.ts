import assert from "node:assert/strict";
import test from "node:test";
import { setupTokenPolicy } from "./setupTokenPolicy.ts";

test("reads the setup token from the URL query string", () => {
  assert.equal(setupTokenPolicy.read("?token=abc123"), "abc123");
  assert.equal(setupTokenPolicy.read("token=abc123"), "abc123");
});

test("ignores a query string that carries no token", () => {
  assert.equal(setupTokenPolicy.read(""), "");
  assert.equal(setupTokenPolicy.read("?"), "");
  assert.equal(setupTokenPolicy.read("?other=value"), "");
});

test("reads the token alongside unrelated query parameters", () => {
  assert.equal(setupTokenPolicy.read("?other=value&token=abc123"), "abc123");
});

test("strips the token from the address without losing other query parameters", () => {
  assert.equal(setupTokenPolicy.strippedUrl("/", ""), "/");
  assert.equal(setupTokenPolicy.strippedUrl("/setup", "?token=abc123"), "/setup");
  assert.equal(
    setupTokenPolicy.strippedUrl("/setup", "?token=abc123&return_to=%2Fchat"),
    "/setup?return_to=%2Fchat",
  );
});
