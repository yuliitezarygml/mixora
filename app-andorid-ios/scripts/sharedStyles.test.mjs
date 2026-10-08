import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, existsSync } from "node:fs";
import { sharedStyleTags } from "./sharedStyles.mjs";

test("mobile build uses the same local styles as the existing client, in the same order", () => {
  const client = new URL("../../mixora-client/", import.meta.url);
  const html = readFileSync(new URL("index.html", client), "utf8");
  const tags = sharedStyleTags(html);
  assert.equal(tags.length, 43);
  assert.equal(tags.at(-1).attrs.href, "/styles/fonts.css");
  for (const tag of tags) {
    assert.equal(tag.tag, "link");
    assert.equal(tag.injectTo, "head");
    assert.ok(existsSync(new URL(`public${tag.attrs.href}`, client)));
  }
});

test("missing reference markers fail the build instead of silently losing the design", () => {
  assert.throws(() => sharedStyleTags("<html></html>"), /reference-styles/);
});
