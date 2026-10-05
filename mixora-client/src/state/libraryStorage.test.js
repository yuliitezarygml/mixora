import assert from "node:assert/strict";
import test from "node:test";
import {
  emptyLibrary,
  transitionSafeLibraryPayload,
  withPins,
} from "./libraryStorage.js";

test("library storage upgrades legacy playlist pins without losing provider identity", () => {
  const library = {
    ...emptyLibrary(),
    playlists: [{ id: "private-mix", name: "Private mix" }],
    savedPlaylists: [
      { source: "spotify", id: "shared-mix", name: "Shared mix" },
    ],
    pins: [
      "private-mix",
      "shared-mix",
      { source: "bandcamp", id: "collection" },
      "spotify:shared-mix",
      "",
    ],
  };

  assert.deepEqual(withPins(library).pins, [
    "mixora:private-mix",
    "spotify:shared-mix",
    "bandcamp:collection",
  ]);
});

test("transition library snapshot never writes dedicated server-owned collections", () => {
  const payload = transitionSafeLibraryPayload({
    ...emptyLibrary(),
    likes: [{ source: "spotify", id: "like" }],
    dislikes: [{ source: "youtube", id: "dislike" }],
    history: [{ source: "soundcloud", id: "history" }],
    playlists: [{ id: "playlist", tracks: [{ id: "track" }] }],
    artists: [{ source: "spotify", id: "artist" }],
  });

  assert.deepEqual(payload.likes, []);
  assert.deepEqual(payload.dislikes, []);
  assert.deepEqual(payload.history, []);
  assert.deepEqual(payload.playlists, []);
  assert.deepEqual(payload.artists, [{ source: "spotify", id: "artist" }]);
});
