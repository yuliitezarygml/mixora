import test from "node:test";
import assert from "node:assert/strict";
import {
  acknowledgePlaylistMutations,
  canonicalPlaylist,
  createPlaylistDeleteMutation,
  createPlaylistMutation,
  enqueuePlaylistMutation,
  legacyPlaylistBackfill,
  mergePlaylists,
  playlistBackfillMarkerKey,
  playlistBatch,
  playlistQueueKey,
  playlistRequest,
  playlistState,
} from "./playlists.js";

const playlist = (id, overrides = {}) => ({
  id,
  name: `Playlist ${id}`,
  tracks: [
    {
      source: "soundcloud",
      id: "soundcloud:tracks:42",
      title: "Track",
      artist: "Artist",
    },
  ],
  ...overrides,
});

const firstID = "123e4567-e89b-12d3-a456-426614174000";
const secondID = "123e4567-e89b-12d3-a456-426614174001";

test("canonicalizes an ordered Mixora playlist and rejects duplicate tracks", () => {
  const value = canonicalPlaylist(
    playlist(` ${firstID.toUpperCase()} `, { name: " В дороге " }),
  );
  assert.deepEqual(value, {
    id: firstID,
    name: "В дороге",
    tracks: [
      {
        source: "soundcloud",
        id: "42",
        title: "Track",
        artist: "Artist",
      },
    ],
    pinned: false,
    liked: false,
  });
  assert.equal(
    canonicalPlaylist(
      playlist(firstID, {
        tracks: [
          playlist(firstID).tracks[0],
          { ...playlist(firstID).tracks[0], id: "42" },
        ],
      }),
    ),
    null,
  );
});

test("playlist queue keeps the latest desired state for one playlist", () => {
  const initial = createPlaylistMutation(playlist(firstID), {
    id: () => "playlist-first",
  });
  const moved = createPlaylistMutation(
    playlist(firstID, { name: "Moved", pinned: true }),
    { id: () => "playlist-moved" },
  );
  const deleted = createPlaylistDeleteMutation(firstID, {
    id: () => "playlist-delete",
  });
  const queue = enqueuePlaylistMutation(
    enqueuePlaylistMutation([initial], moved),
    deleted,
  );
  assert.deepEqual(queue.map((item) => [item.type, item.idempotency_key]), [
    ["delete", "playlist-delete"],
  ]);
  assert.deepEqual(playlistBatch(queue), queue);
  assert.deepEqual(acknowledgePlaylistMutations(queue, [deleted]), []);
});

test("server state is authoritative while offline playlist edits remain visible", () => {
  const remote = {
    playlists: [playlist(firstID, { name: "Server", pinned: false })],
  };
  const pending = [
    createPlaylistMutation(playlist(firstID, { name: "Local edit", pinned: true }), {
      id: () => "playlist-local",
    }),
    createPlaylistMutation(playlist(secondID, { name: "Offline new" }), {
      id: () => "playlist-new",
    }),
  ];
  const merged = mergePlaylists(
    { playlists: [playlist(secondID, { name: "Stale browser" })], pins: [] },
    remote,
    pending,
  );
  assert.deepEqual(
    merged.playlists.map((item) => [item.id, item.name]),
    [
      [secondID, "Offline new"],
      [firstID, "Local edit"],
    ],
  );
  assert.deepEqual(merged.pins, [firstID]);
});

test("creates precise API requests and only backfills an empty normalized account", () => {
  const mutation = createPlaylistMutation(playlist(firstID), {
    id: () => "playlist-request",
  });
  assert.deepEqual(playlistRequest(mutation), {
    method: "PUT",
    path: `/me/playlists/${firstID}`,
    body: {
      idempotency_key: "playlist-request",
      name: `Playlist ${firstID}`,
      description: "",
      tracks: [
        { source: "soundcloud", id: "42", title: "Track", artist: "Artist" },
      ],
      pinned: false,
      liked: false,
    },
  });
  const backfill = legacyPlaylistBackfill(
    { playlists: [playlist(firstID)], pins: [firstID] },
    { create: (value) => ({ type: "replace", idempotency_key: "legacy", playlist: value }) },
  );
  assert.equal(backfill.length, 1);
  assert.equal(backfill[0].playlist.pinned, true);
  assert.deepEqual(playlistState({ playlists: [playlist(firstID)] }).map((item) => item.id), [firstID]);
  assert.equal(playlistQueueKey("user-1"), "mixora-ui:playlist-queue:user-1");
  assert.equal(playlistBackfillMarkerKey("user-1"), "mixora-ui:playlist-backfill-v1:user-1");
});
