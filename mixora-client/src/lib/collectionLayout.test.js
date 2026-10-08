import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

// Public CSS seam: intrinsic image sizes / long artist names must never size
// carousel items. The corresponding rendered-width check is in design-qa.md.
const css = readFileSync(
  new URL("../styles/integration.css", import.meta.url),
  "utf8",
);
test("collection carousel cards have a bounded width independent of metadata", () => {
  const rule =
    css.match(/\.collection-carousel > div\s*\{([^}]+)\}/)?.[1] || "";
  assert.match(rule, /flex:\s*0 0 var\(--collection-tile-width\)/);
  assert.match(rule, /width:\s*var\(--collection-tile-width\)/);
  assert.match(rule, /min-width:\s*0/);
});
test("collection playlist and artist metadata can shrink and truncate", () => {
  const rule = css.match(/\.collection-playlist\s*\{([^}]+)\}/)?.[1] || "";
  assert.match(rule, /min-width:\s*0/);
  assert.match(
    css,
    /\.collection-playlist > a\s*\{[^}]*text-overflow:\s*ellipsis/,
  );
});
test("recommended artwork does not grow with a wide desktop window", () => {
  const rule = css.match(/\.recommended-grid\s*\{([^}]+)\}/)?.[1] || "";
  assert.match(
    rule,
    /grid-template-columns:\s*repeat\(3, minmax\(0, 220px\)\)/,
  );
});
