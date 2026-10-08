import test from "node:test";
import assert from "node:assert/strict";
import { api, getTrackPlayback } from "./api.js";
import {
  configureClientRuntime,
  createPlaybackSocket,
} from "./clientRuntime.js";

test("native transport receives the existing API contract, browser default is restored", async () => {
  const calls = [];
  const controller = new AbortController();
  const restore = configureClientRuntime({
    request: async (url, options) => {
      calls.push({ url, options });
      return new Response(
        JSON.stringify({ success: true, data: { id: "native-user" } }),
      );
    },
  });
  try {
    assert.deepEqual(await api("/me", { signal: controller.signal }), {
      id: "native-user",
    });
    assert.equal(calls[0].url, "/api/v1/me");
    assert.equal(calls[0].options.signal, controller.signal);
    assert.equal(calls[0].options.credentials, "include");
  } finally {
    restore();
  }
});

test("authenticated media crosses the runtime boundary instead of a broken WebView URL", async () => {
  const prepared = [];
  const controller = new AbortController();
  const restore = configureClientRuntime({
    preparePlayback: async (playback, signal) => {
      prepared.push({ playback, signal });
      return { ...playback, url: "asset://native-audio" };
    },
  });
  try {
    const result = await getTrackPlayback(
      {
        source: "youtube",
        id: "abc",
        permalink: "https://www.youtube.com/watch?v=abc",
      },
      controller.signal,
    );
    assert.equal(result.url, "asset://native-audio");
    assert.equal(prepared[0].signal, controller.signal);
    assert.match(prepared[0].playback.url, /^\/api\/v1\/media\/stream\?/);
  } finally {
    restore();
  }
});

test("native runtimes can explicitly disable browser WebSocket until cookie-aware sync exists", () => {
  const restore = configureClientRuntime({ createPlaybackSocket: () => null });
  try {
    assert.equal(
      createPlaybackSocket({ protocol: "tauri:", host: "localhost" }),
      null,
    );
  } finally {
    restore();
  }
});
