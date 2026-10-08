import assert from "node:assert/strict";
import { test } from "node:test";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { useRecommendedPlaylists } from "./useRecommendedPlaylists.js";

const track = {
  source: "youtube",
  id: "one",
  title: "Music",
  artist: "Artist",
};
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
  const timers = new Map();
  let timer = 0;
  t.mock.method(window, "setTimeout", (callback, delay) => {
    const id = ++timer;
    timers.set(id, { callback, delay });
    return id;
  });
  t.mock.method(window, "clearTimeout", (id) => timers.delete(id));
  t.mock.method(globalThis, "fetch", fetcher);
  let state;
  function Capture({ options }) {
    state = useRecommendedPlaylists(options);
    return null;
  }
  const root = createRoot(document.getElementById("root"));
  const render = async (options) =>
    act(async () => root.render(createElement(Capture, { options })));
  const flush = async () =>
    act(async () => {
      const jobs = [...timers].filter(([, job]) => job.delay === 600);
      for (const [id, job] of jobs) {
        timers.delete(id);
        await job.callback();
      }
    });
  t.after(async () => {
    await act(async () => root.unmount());
    dom.window.close();
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else delete globalThis[key];
    }
  });
  return { render, flush, state: () => state, timers };
}
const response = () =>
  new Response(
    JSON.stringify({
      tracks: [track],
      session_id: "session",
      model_version: "gorse-v1",
    }),
    { headers: { "Content-Type": "application/json" } },
  );
test("recommendations debounce, load three mixes and refresh only when taste changes", async (t) => {
  let calls = 0;
  const app = await mount(t, async () => {
    calls++;
    return response();
  });
  const options = { userId: "a", library: {}, catalog: [track] };
  await app.render(options);
  assert.equal(app.state().loading, true);
  assert.equal(calls, 0);
  await app.flush();
  assert.equal(calls, 3);
  assert.equal(app.state().playlists.length, 3);
  await app.render({ ...options, position: 100 });
  await app.flush();
  assert.equal(calls, 3);
  await app.render({ ...options, library: { likes: [track] } });
  assert.equal(app.state().playlists.length, 0);
  await app.flush();
  assert.equal(calls, 6);
  assert.equal(app.state().playlists.length, 2);
});
test("logging out fences visible recommendations and cancels scheduled loads", async (t) => {
  const app = await mount(t, async () => response());
  await app.render({ userId: "a" });
  await app.flush();
  assert.equal(app.state().playlists.length, 3);
  await app.render({ userId: null });
  assert.equal(app.state().playlists.length, 0);
  assert.equal(app.state().loading, false);
  assert.equal(app.timers.size, 0);
});
test("late responses from a different account never replace the new account's mixes", async (t) => {
  const pending = [];
  const app = await mount(
    t,
    (url, options) =>
      new Promise((resolve) =>
        pending.push({ resolve, signal: options.signal }),
      ),
  );
  await app.render({ userId: "a" });
  // Start the debounce job without awaiting its outstanding network request.
  await act(async () => {
    for (const [id, job] of app.timers)
      if (job.delay === 600) {
        app.timers.delete(id);
        void job.callback();
      }
  });
  await app.render({ userId: "b" });
  assert.equal(app.state().playlists.length, 0);
  await act(async () => {
    for (const job of pending) job.resolve(response());
  });
  assert.equal(app.state().playlists.length, 0);
  assert.ok(pending.every((job) => job.signal.aborted));
});
