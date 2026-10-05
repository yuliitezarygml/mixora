import test from "node:test";
import assert from "node:assert/strict";
import {
  duration,
  entityKey,
  findKnownTrack,
  trackKey,
  uniqueTracks,
  shuffleTracks,
  libraryPayload,
  libraryCount,
  saveStorage,
} from "./library.js";
test("duration remains valid before metadata loads", () => {
  assert.equal(duration(NaN), "0:00");
  assert.equal(duration(125), "2:05");
});
test("provider IDs remain distinct", () => {
  assert.equal(
    uniqueTracks([
      { source: "local", id: "1" },
      { source: "soundcloud", id: "1" },
      { source: "local", id: "1" },
    ]).length,
    2,
  );
});
test("track key ignores a query-only history entry", () => {
  assert.equal(trackKey(undefined), "");
  assert.equal(trackKey({ source: "spotify", id: "same" }), "spotify:same");
});
test("provider-aware entity keys preserve legacy SoundCloud saves", () => {
  assert.equal(entityKey({ source: "spotify", id: "same" }), "spotify:same");
  assert.equal(entityKey({ source: "bandcamp", id: "same" }), "bandcamp:same");
  assert.equal(entityKey({ id: "legacy" }), "soundcloud:legacy");
});
test("known tracks survive a reload through provider-aware playlist snapshots", () => {
  const bandcamp = {
    source: "bandcamp",
    id: "release-1",
    title: "Saved release",
    artist: "Artist",
    permalink: "https://artist.bandcamp.com/track/saved-release",
  };
  const found = findKnownTrack(
    [{ source: "soundcloud", id: "release-1", title: "Different track" }],
    { playlists: [{ id: "playlist", tracks: [bandcamp] }] },
    "bandcamp",
    "release-1",
  );
  assert.deepEqual(found, bandcamp);
});
test("library payload keeps server limits", () => {
  const payload = libraryPayload({
    likes: Array.from({ length: 500 }, (_, i) => ({ id: i })),
    playlists: [{ id: "p", tracks: Array.from({ length: 250 }, (_, i) => i) }],
  });
  assert.equal(payload.likes.length, 400);
  assert.equal(payload.playlists[0].tracks.length, 200);
  assert.equal(libraryCount(payload), 401);
});
test("shuffle keeps current track and every remaining track", () => {
  const tracks = [{ id: 1 }, { id: 2 }, { id: 3 }];
  const result = shuffleTracks(tracks, 1, () => 0);
  assert.equal(result[0].id, 2);
  assert.deepEqual(result.map((t) => t.id).sort(), [1, 2, 3]);
  assert.deepEqual(
    tracks.map((t) => t.id),
    [1, 2, 3],
  );
});
test("storage writes report whether a durable queue could be saved", () => {
  const original = globalThis.localStorage;
  try {
    globalThis.localStorage = {
      setItem() {},
    };
    assert.equal(saveStorage("mixora-test", { queued: true }), true);
    globalThis.localStorage = {
      setItem() {
        throw Error("storage is full");
      },
    };
    assert.equal(saveStorage("mixora-test", { queued: true }), false);
  } finally {
    if (original === undefined) delete globalThis.localStorage;
    else globalThis.localStorage = original;
  }
});
