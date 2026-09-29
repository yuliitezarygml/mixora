import test from "node:test";
import assert from "node:assert/strict";
import { dropAccountToken, rememberAccount } from "./accounts.js";

test("remembered accounts stay on one list and the latest login is first", () => {
  const first = rememberAccount([], {
    id: "a",
    email: "a@example.test",
    display_name: "Аня",
    plus: false,
  });
  const next = rememberAccount(first, {
    id: "b",
    email: "b@example.test",
    display_name: "Боря",
    plus: true,
  });
  const again = rememberAccount(next, {
    id: "a",
    email: "a@example.test",
    display_name: "Аня",
    plus: true,
  });
  assert.deepEqual(
    again.map((item) => item.id),
    ["a", "b"],
  );
  assert.equal(again[0].plus, true);
  assert.equal(rememberAccount(null, null).length, 0);
});

test("each account keeps its own token when another account signs in", () => {
  const first = rememberAccount(
    [],
    {
      id: "a",
      email: "a@example.test",
      display_name: "Аня",
      plus: false,
    },
    "token-a",
  );
  const next = rememberAccount(
    first,
    {
      id: "b",
      email: "b@example.test",
      display_name: "Боря",
      plus: true,
    },
    "token-b",
  );
  assert.equal(
    next.find((item) => item.id === "a").token,
    "token-a",
  );
  assert.equal(
    next.find((item) => item.id === "b").token,
    "token-b",
  );
  const refreshed = rememberAccount(next, {
    id: "a",
    email: "a@example.test",
    display_name: "Аня",
    plus: true,
  });
  assert.equal(
    refreshed.find((item) => item.id === "a").token,
    "token-a",
  );
  const signedOut = dropAccountToken(refreshed, "b");
  assert.equal(
    signedOut.find((item) => item.id === "b").token,
    undefined,
  );
  assert.equal(
    signedOut.find((item) => item.id === "a").token,
    "token-a",
  );
});
