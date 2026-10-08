import assert from "node:assert/strict";
import { test } from "node:test";
import {
  recommendationFingerprint,
  recommendationRequest,
  loadRecommendedPlaylists,
} from "./recommendedPlaylists.js";

const tracks = ["soundcloud", "youtube", "vk", "bandcamp", "spotify"].map(
  (source) => ({
    source,
    id: "1",
    title: source,
    artist: "Artist",
    access: source === "spotify" ? "preview" : "full",
  }),
);
test("recommendation presets keep all sources and fresh taste signals", () => {
  const request = recommendationRequest("discover", {
    library: {
      likes: [tracks[0]],
      history: [tracks[1]],
      dislikes: [tracks[2]],
    },
    catalog: tracks,
    preferences: { language: "russian" },
    explicit: false,
  });
  assert.equal(request.preferences.diversity, "unknown");
  assert.equal(request.preferences.language, "russian");
  assert.equal(request.explicit, false);
  assert.deepEqual(request.likes, [tracks[0]]);
  assert.deepEqual(request.dislikes, [tracks[2]]);
  assert.equal(request.seeds.length, 5);
});
test("taste fingerprint changes for likes, searches and account but not player position", () => {
  const base = {
    userId: "a",
    library: {
      likes: [tracks[0]],
      history: [tracks[1]],
      searches: [{ q: "ODESZA" }],
    },
    catalog: tracks,
  };
  const key = recommendationFingerprint(base);
  assert.equal(recommendationFingerprint({ ...base, position: 200 }), key);
  assert.notEqual(recommendationFingerprint({ ...base, userId: "b" }), key);
  assert.notEqual(
    recommendationFingerprint({
      ...base,
      library: { ...base.library, likes: [tracks[2]] },
    }),
    key,
  );
  assert.notEqual(
    recommendationFingerprint({
      ...base,
      library: { ...base.library, searches: [{ q: "Tycho" }] },
    }),
    key,
  );
});
test("recent searched tracks seed discovery without turning them into likes", () => {
  const searched = {
    ...tracks[1],
    id: "recent",
    title: "A Moment Apart",
    artist: "ODESZA",
  };
  const options = {
    library: { searches: [{ query: "ODESZA", source: "youtube" }] },
    catalog: [tracks[0], searched],
  };
  const request = recommendationRequest("discover", options);
  assert.equal(request.context.artist, "ODESZA");
  assert.equal(request.seeds[0].id, "recent");
  assert.deepEqual(request.likes, []);
  const focus = recommendationRequest("focus", options);
  assert.equal(focus.preferences.activity, "work");
  assert.equal(focus.context.artist, "ODESZA");
});
test("returned playlists exclude dislikes, blocked, explicit, malformed and duplicate tracks", async () => {
  const options = {
    userId: "a",
    library: { dislikes: [tracks[2]] },
    explicit: false,
  };
  const request = async () => ({
    tracks: [
      ...tracks,
      tracks[0],
      { ...tracks[1], id: "blocked", access: "blocked" },
      { ...tracks[1], id: "adult", explicit: true },
      { title: "broken" },
    ],
    session_id: "session",
    model_version: "gorse-v1",
  });
  const result = await loadRecommendedPlaylists(options, request);
  assert.equal(result.playlists.length, 3);
  for (const item of result.playlists) {
    assert.deepEqual(
      item.tracks.map((t) => t.source),
      ["soundcloud", "youtube", "bandcamp", "spotify"],
    );
    assert.equal(item.owner, "a");
    assert.equal(item.sessionId, "session");
  }
});
test("discovery does not repeat tracks already known to the listener", async () => {
  const result = await loadRecommendedPlaylists(
    { userId: "a", library: { likes: [tracks[0]], history: [tracks[1]] } },
    async () => ({ tracks }),
  );
  assert.deepEqual(
    result.playlists
      .find((p) => p.id === "discover")
      .tracks.map((t) => t.source),
    ["vk", "bandcamp", "spotify"],
  );
});
test("a failed mix keeps successful mixes and reports partial failure", async () => {
  const result = await loadRecommendedPlaylists(
    { userId: "a" },
    async (path, options) => {
      if (JSON.parse(options.body).preferences.activity === "work")
        throw new Error("unavailable");
      return { tracks };
    },
  );
  assert.equal(result.playlists.length, 2);
  assert.equal(result.failures, 1);
});

test("a small rule-only catalog explains a focus mix that repeats the daily mix", async () => {
  const result = await loadRecommendedPlaylists({ userId: "a" }, async () => ({
    tracks,
    model_version: "rules-v0",
  }));
  assert.equal(
    result.playlists.find((item) => item.id === "focus").limitedCatalog,
    true,
  );
  assert.equal(
    result.playlists.find((item) => item.id === "daily").limitedCatalog,
    undefined,
  );
});
test("cancelled recommendation requests never return a successful stale result", async () => {
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(
    loadRecommendedPlaylists(
      { userId: "a", signal: controller.signal },
      async () => ({ tracks }),
    ),
    { name: "AbortError" },
  );
});
