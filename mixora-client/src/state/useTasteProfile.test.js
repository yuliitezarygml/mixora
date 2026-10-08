import assert from "node:assert/strict";
import { test } from "node:test";
import { act, createElement, useRef } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { useTasteProfile } from "./useTasteProfile.js";

const profile = (name) => ({
  artists: [name, "Two", "Three", "Four", "Five"],
  genres: [],
  completed: true,
});
const response = (value) => new Response(JSON.stringify(value));
async function mount(t, fetcher) {
  const dom = new JSDOM('<div id="root"></div>');
  const originals = new Map();
  for (const [key, value] of Object.entries({
    window: dom.window,
    document: dom.window.document,
    IS_REACT_ACT_ENVIRONMENT: true,
  })) {
    originals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    Object.defineProperty(globalThis, key, {
      value,
      writable: true,
      configurable: true,
    });
  }
  t.mock.method(globalThis, "fetch", fetcher);
  let state;
  function Capture({ id }) {
    const owner = useRef(null);
    owner.current = id ? { id } : null;
    state = useTasteProfile(id, owner);
    return null;
  }
  const root = createRoot(document.getElementById("root"));
  t.after(async () => {
    await act(async () => root.unmount());
    dom.window.close();
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else delete globalThis[key];
    }
  });
  return {
    render: (id) =>
      act(async () => root.render(createElement(Capture, { id }))),
    state: () => state,
  };
}

test("late onboarding profile never leaks to another account or after logout", async (t) => {
  const pending = [];
  const app = await mount(
    t,
    (url, options) =>
      new Promise((resolve) =>
        pending.push({ resolve, signal: options.signal }),
      ),
  );
  await app.render("a");
  await app.render("b");
  assert.equal(pending[0].signal.aborted, true);
  await act(async () => pending[1].resolve(response(profile("B"))));
  await act(async () => pending[0].resolve(response(profile("A"))));
  assert.equal(app.state().profile.artists[0], "B");
  await app.render("");
  assert.deepEqual(app.state().profile.artists, []);
  assert.equal(app.state().open, false);
});

test("late initial GET cannot undo a newly saved taste profile", async (t) => {
  let get;
  const app = await mount(t, (url, options) =>
    options.method === "PUT"
      ? Promise.resolve(response(profile("Saved")))
      : new Promise((resolve) => {
          get = resolve;
        }),
  );
  await app.render("a");
  await act(async () =>
    assert.equal(await app.state().save(profile("Saved")), true),
  );
  await act(async () =>
    get(response({ artists: [], genres: [], completed: false })),
  );
  assert.equal(app.state().profile.artists[0], "Saved");
  assert.equal(app.state().open, false);
});

test("a save response for the previous account does not replace the new profile", async (t) => {
  let save;
  let gets = 0;
  const app = await mount(t, (url, options) =>
    options.method === "PUT"
      ? new Promise((resolve) => {
          save = resolve;
        })
      : Promise.resolve(response(profile(++gets === 1 ? "A" : "B"))),
  );
  await app.render("a");
  let saving;
  await act(async () => {
    saving = app.state().save(profile("Changed A"));
  });
  await app.render("b");
  await act(async () => save(response(profile("Changed A"))));
  assert.equal(await saving, false);
  assert.equal(app.state().profile.artists[0], "B");
});
