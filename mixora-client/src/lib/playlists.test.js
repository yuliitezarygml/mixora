import test from "node:test";
import assert from "node:assert/strict";
import {
  acknowledgePlaylistMutations,
  canonicalPlaylist,
  createPlaylistDeleteMutation,
  createPlaylistMutation,
  discardPlaylistMutations,
  enqueuePlaylistMutation,
  legacyPlaylistBackfill,
  mergePlaylists,
  playlistBackfillMarkerKey,
  playlistBatch,
  playlistQueueKey,
  playlistRequest,
  playlistState,
  rebasePlaylistMutations,
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
const thirdID = "123e4567-e89b-12d3-a456-426614174002";

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
    revision: 0,
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
  assert.deepEqual(
    queue.map((item) => [item.type, item.idempotency_key]),
    [["delete", "playlist-delete"]],
  );
  assert.deepEqual(playlistBatch(queue), queue);
  assert.deepEqual(acknowledgePlaylistMutations(queue, [deleted]), []);
});

test("server state is authoritative while offline playlist edits remain visible", () => {
  const remote = {
    playlists: [playlist(firstID, { name: "Server", pinned: false })],
  };
  const pending = [
    createPlaylistMutation(
      playlist(firstID, { name: "Local edit", pinned: true }),
      {
        id: () => "playlist-local",
      },
    ),
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
  assert.deepEqual(merged.pins, [`mixora:${firstID}`]);
});

test("creates precise API requests and backfills only unrepresented legacy playlists", () => {
  const mutation = createPlaylistMutation(playlist(firstID), {
    id: () => "playlist-request",
  });
  assert.deepEqual(playlistRequest(mutation), {
    method: "PUT",
    path: `/me/playlists/${firstID}`,
    body: {
      idempotency_key: "playlist-request",
      expected_revision: 0,
      name: `Playlist ${firstID}`,
      description: "",
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
    },
  });
  const backfill = legacyPlaylistBackfill(
    {
      playlists: [
        playlist(firstID),
        playlist("legacy-browser", { name: "Offline P" }),
      ],
      pins: [firstID],
    },
    {
      remote: {
        playlists: [
          playlist(firstID),
          playlist(secondID, { legacy_id: "already-migrated" }),
        ],
      },
      generateID: () => thirdID,
      create: (value) => ({
        type: "replace",
        idempotency_key: "legacy",
        playlist: value,
      }),
    },
  );
  assert.equal(backfill.length, 1);
  assert.equal(backfill[0].playlist.id, thirdID);
  assert.equal(backfill[0].playlist.name, "Offline P");
  assert.equal(backfill[0].playlist.pinned, false);
  const migrated = legacyPlaylistBackfill(
    { playlists: [playlist("already-migrated")], pins: [] },
    {
      remote: {
        playlists: [playlist(secondID, { legacy_id: "already-migrated" })],
      },
      generateID: () => firstID,
    },
  );
  assert.deepEqual(migrated, []);
  assert.deepEqual(
    playlistState({ playlists: [playlist(firstID)] }).map((item) => item.id),
    [firstID],
  );
  assert.equal(playlistQueueKey("user-1"), "mixora-ui:playlist-queue:user-1");
  assert.equal(
    playlistBackfillMarkerKey("user-1"),
    "mixora-ui:playlist-backfill-v1:user-1",
  );
});

test("preserves optimistic revision intent and marks browser snapshot backfills", () => {
  const current = playlist(firstID, { revision: 4 });
  const next = createPlaylistMutation(
    { ...current, name: "Edited from this device" },
    { id: () => "playlist-edit" },
  );
  const remove = createPlaylistDeleteMutation(current, {
    id: () => "playlist-delete",
  });
  assert.deepEqual(playlistRequest(next).body.expected_revision, 4);
  assert.deepEqual(playlistRequest(remove).body.expected_revision, 4);
  const rebased = rebasePlaylistMutations([next, remove], firstID, 5);
  assert.deepEqual(
    rebased.map((mutation) =>
      mutation.type === "replace"
        ? [mutation.type, mutation.playlist.revision]
        : [mutation.type, mutation.expected_revision],
    ),
    [["delete", 5]],
  );
  assert.deepEqual(discardPlaylistMutations(rebased, firstID), []);

  const backfill = legacyPlaylistBackfill(
    { playlists: [playlist("legacy-browser")], pins: [] },
    {
      remote: { playlists: [] },
      generateID: () => secondID,
    },
  );
  assert.equal(backfill.length, 1);
  assert.equal(backfill[0].legacy_id, "legacy-browser");
  assert.equal(playlistRequest(backfill[0]).body.legacy_id, "legacy-browser");
});
