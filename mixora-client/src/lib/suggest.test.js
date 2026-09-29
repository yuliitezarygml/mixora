import test from "node:test";
import assert from "node:assert/strict";
import { correctQuery, tasteQuery } from "./suggest.js";

test("search fixes a short typo against known names", () => {
  assert.equal(correctQuery("miygi", ["Miyagi", "Tycho"]), "Miyagi");
  assert.equal(
    correctQuery("miygi", ["Endorphin (feat. Miyagi)"]),
    "Miyagi",
  );
  assert.equal(correctQuery("ty", ["Tycho"]), "ty");
});

test("wave taste follows what the listener searches and plays", () => {
  assert.equal(
    tasteQuery({
      searches: ["ODESZA", "ODESZA"],
      history: [{ artist: "Tycho" }],
      likes: [],
    }),
    "ODESZA",
  );
});
