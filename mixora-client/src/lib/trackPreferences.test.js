import test from "node:test";
import assert from "node:assert/strict";
import {
  acknowledgeTrackPreferences,
  applyTrackPreference,
  createTrackPreferenceMutation,
  enqueueTrackPreference,
  legacyTrackPreferenceBackfill,
  mergeTrackPreferences,
  preferenceBatch,
  preserveDisplacedTrackPreferenceMigrations,
  refillTrackPreferenceQueue,
  removeTrackPreferenceMutation,
  trackPreferenceBackfillKey,
  trackPreferenceBackfillMarkerKey,
  trackPreferenceQueueKey,
  trackPreferenceRequest,
  trackPreferenceState,
} from "./trackPreferences.js";

const track = (id, overrides = {}) => ({
  id: String(id),
  source: "music",
  title: `Track ${id}`,
  artist: `Artist ${id}`,
  ...overrides,
});

test("creates a canonical SoundCloud preference mutation", () => {
  assert.deepEqual(
    createTrackPreferenceMutation(
      {
        id: "soundcloud:tracks:1534086151",
        source: "SoundCloud",
        title: "Тестовый трек",
        artist: "Тестовый исполнитель",
      },
      "liked",
      { id: () => "preference-1" },
    ),
    {
      idempotency_key: "preference-1",
      track: {
        id: "1534086151",
        source: "soundcloud",
        title: "Тестовый трек",
        artist: "Тестовый исполнитель",
      },
      preference: "liked",
    },
  );
});

test("rejects invalid preference states and incomplete track references", () => {
  assert.equal(
    createTrackPreferenceMutation(
      { id: "42", source: "soundcloud", title: "Track", artist: "Artist" },
      "favorite",
      { id: () => "invalid-state" },
    ),
    null,
  );
  assert.equal(
    createTrackPreferenceMutation(
      {
        id: "not-a-soundcloud-id",
        source: "soundcloud",
        title: "Track",
        artist: "Artist",
      },
      "liked",
      { id: () => "invalid-track" },
    ),
    null,
  );
  assert.equal(
    createTrackPreferenceMutation(
      { source: "soundcloud", title: "Track", artist: "Artist" },
      "liked",
      { id: () => "missing-track" },
    ),
    null,
  );
  assert.equal(
    createTrackPreferenceMutation(
      { id: "42", source: "soundcloud", title: "Track" },
      "liked",
      { id: () => "missing-artist" },
    ),
    null,
  );
});

test("offline queue keeps only the latest desired state for one track and stays bounded", () => {
  const unrelated = createTrackPreferenceMutation(
    { id: "7", source: "soundcloud", title: "Seven", artist: "Artist" },
    "liked",
    { id: () => "other" },
  );
  const latest = createTrackPreferenceMutation(
    { id: "42", source: "soundcloud", title: "Forty two", artist: "Artist" },
    "disliked",
    { id: () => "latest" },
  );
  const queued = enqueueTrackPreference(
    [
      {
        idempotency_key: "legacy-liked",
        track: {
          id: "soundcloud:tracks:42",
          source: "SoundCloud",
          title: "Forty two",
          artist: "Artist",
        },
        preference: "liked",
      },
      unrelated,
    ],
    latest,
  );

  assert.deepEqual(
    queued.map((item) => item.idempotency_key),
    ["other", "latest"],
  );
  assert.equal(queued[1].preference, "disliked");

  const bounded = enqueueTrackPreference(
    queued,
    createTrackPreferenceMutation(
      { id: "8", source: "soundcloud", title: "Eight", artist: "Artist" },
      "neutral",
      { id: () => "third" },
    ),
    2,
  );
  assert.deepEqual(
    bounded.map((item) => item.idempotency_key),
    ["latest", "third"],
  );
});

test("acknowledges only delivered actions without removing a newer state", () => {
  const delivered = createTrackPreferenceMutation(
    { id: "42", source: "soundcloud", title: "Forty two", artist: "Artist" },
    "liked",
    { id: () => "sent-liked" },
  );
  const newer = createTrackPreferenceMutation(
    { id: "42", source: "soundcloud", title: "Forty two", artist: "Artist" },
    "neutral",
    { id: () => "new-neutral" },
  );
  const other = createTrackPreferenceMutation(
    { id: "9", source: "soundcloud", title: "Nine", artist: "Artist" },
    "disliked",
    { id: () => "other" },
  );
  const queue = enqueueTrackPreference(
    enqueueTrackPreference([delivered, other], newer),
    null,
  );

  assert.deepEqual(
    preferenceBatch(queue, 1).map((item) => item.idempotency_key),
    ["other"],
  );
  assert.deepEqual(
    acknowledgeTrackPreferences(queue, [delivered]).map(
      (item) => item.idempotency_key,
    ),
    ["other", "new-neutral"],
  );
  assert.deepEqual(
    acknowledgeTrackPreferences(queue, [newer]).map(
      (item) => item.idempotency_key,
    ),
    ["other"],
  );
});

test("acknowledging an old persisted queue retains its configured bound", () => {
  const oversized = Array.from({ length: 101 }, (_, index) => ({
    idempotency_key: `queued-${index}`,
    track: {
      id: String(index),
      source: "soundcloud",
      title: `Track ${index}`,
      artist: "Artist",
    },
    preference: "liked",
  }));

  const remaining = acknowledgeTrackPreferences(oversized, []);
  assert.equal(remaining.length, 100);
  assert.equal(remaining[0].idempotency_key, "queued-1");
  assert.equal(remaining.at(-1).idempotency_key, "queued-100");
});

test("server state is authoritative but unsent local mutations win", () => {
  const library = {
    likes: [track("stale-like")],
    dislikes: [track("stale-dislike")],
    history: [track("history")],
  };
  const server = trackPreferenceState({
    preferences: [
      { track: track("1"), preference: "liked", revision: 3 },
      { track: track("2"), preference: "disliked", revision: 4 },
      { track: track("stale-like"), preference: "neutral", revision: 5 },
    ],
  });
  const pending = [
    createTrackPreferenceMutation(track("1"), "neutral", {
      id: () => "pending-neutral",
    }),
    createTrackPreferenceMutation(track("3"), "liked", {
      id: () => "pending-liked",
    }),
  ];

  const merged = mergeTrackPreferences(library, server, pending);
  assert.deepEqual(
    merged.likes.map((value) => value.id),
    ["3"],
  );
  assert.deepEqual(
    merged.dislikes.map((value) => value.id),
    ["2"],
  );
  assert.deepEqual(
    merged.history.map((value) => value.id),
    ["history"],
  );
});

test("a successful preference response cannot overwrite a newer pending state", () => {
  const current = applyTrackPreference(
    { likes: [], dislikes: [] },
    { track: track("42"), preference: "liked" },
  );
  const pending = [
    createTrackPreferenceMutation(track("42"), "disliked", {
      id: () => "newer-dislike",
    }),
  ];

  const merged = mergeTrackPreferences(
    current,
    [{ track: track("42"), preference: "liked" }],
    pending,
    { authoritative: false },
  );
  assert.deepEqual(merged.likes, []);
  assert.deepEqual(
    merged.dislikes.map((value) => value.id),
    ["42"],
  );
});

test("legacy browser likes and dislikes become compact current-state writes", () => {
  let sequence = 0;
  const backfill = legacyTrackPreferenceBackfill(
    {
      likes: [
        track("liked"),
        track("both"),
        { id: "missing", source: "music" },
      ],
      dislikes: [track("disliked"), track("both")],
    },
    { id: () => `migration-${++sequence}` },
  );

  assert.deepEqual(
    backfill.map((mutation) => [mutation.track.id, mutation.preference]),
    [
      ["liked", "liked"],
      ["both", "disliked"],
      ["disliked", "disliked"],
    ],
  );
  assert.deepEqual(
    backfill.map((mutation) => mutation.idempotency_key),
    ["migration-2", "migration-3", "migration-4"],
  );
  assert.ok(backfill.every((mutation) => mutation.migration === true));
  assert.deepEqual(trackPreferenceRequest(backfill[0]), {
    idempotency_key: "migration-2",
    track: backfill[0].track,
    preference: "liked",
  });
});

test("legacy backfill never evicts a newer normal mutation and drains in order", () => {
  const manual = createTrackPreferenceMutation(track("manual"), "liked", {
    id: () => "manual",
  });
  const migration = legacyTrackPreferenceBackfill(
    {
      likes: [track("first"), track("manual"), track("second")],
      dislikes: [],
    },
    {
      id: (() => {
        let sequence = 0;
        return () => `migration-${++sequence}`;
      })(),
    },
  );
  const first = refillTrackPreferenceQueue([manual], migration, 2);

  assert.deepEqual(
    first.queue.map((mutation) => mutation.track.id),
    ["manual", "second"],
  );
  assert.deepEqual(
    first.backfill.map((mutation) => mutation.track.id),
    ["first"],
  );

  const second = refillTrackPreferenceQueue([], first.backfill, 2);
  assert.deepEqual(
    second.queue.map((mutation) => mutation.track.id),
    ["first"],
  );
  assert.deepEqual(second.backfill, []);
});

test("a full queue returns only displaced migration records to the bridge", () => {
  const migration = legacyTrackPreferenceBackfill(
    { likes: [track("oldest"), track("newer")], dislikes: [] },
    {
      id: (() => {
        let sequence = 0;
        return () => `migration-${++sequence}`;
      })(),
    },
  );
  const manual = createTrackPreferenceMutation(track("manual"), "liked", {
    id: () => "manual",
  });
  const previousQueue = migration.slice();
  const nextQueue = enqueueTrackPreference(previousQueue, manual, 2);

  assert.deepEqual(
    nextQueue.map((mutation) => mutation.track.id),
    ["oldest", "manual"],
  );
  assert.deepEqual(
    preserveDisplacedTrackPreferenceMigrations(
      [],
      previousQueue,
      nextQueue,
      manual.track,
    ).map((mutation) => mutation.track.id),
    ["newer"],
  );
});

test("a new local choice removes only its matching legacy bridge entry", () => {
  const bridge = legacyTrackPreferenceBackfill(
    { likes: [track("one"), track("two")], dislikes: [] },
    {
      id: (() => {
        let sequence = 0;
        return () => `migration-${++sequence}`;
      })(),
    },
  );

  assert.deepEqual(
    removeTrackPreferenceMutation(bridge, track("one")).map(
      (mutation) => mutation.track.id,
    ),
    ["two"],
  );
  assert.deepEqual(
    removeTrackPreferenceMutation(bridge, track("unknown")).map(
      (mutation) => mutation.track.id,
    ),
    ["two", "one"],
  );
});

test("preference queue is isolated per account", () => {
  assert.equal(
    trackPreferenceQueueKey("user-a"),
    "mixora-ui:track-preferences:user-a",
  );
  assert.notEqual(
    trackPreferenceQueueKey("user-a"),
    trackPreferenceQueueKey("user-b"),
  );
  assert.equal(
    trackPreferenceBackfillKey("user-a"),
    "mixora-ui:track-preference-backfill:user-a",
  );
  assert.equal(
    trackPreferenceBackfillMarkerKey("user-a"),
    "mixora-ui:track-preference-backfill-v1:user-a",
  );
  assert.notEqual(
    trackPreferenceBackfillKey("user-a"),
    trackPreferenceBackfillKey("user-b"),
  );
});
