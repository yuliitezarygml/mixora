import test from "node:test";
import assert from "node:assert/strict";
import {
  acknowledgeHistoryRecords,
  bindHistoryGeneration,
  canonicalHistoryTrack,
  createHistoryRecord,
  enqueueHistoryRecord,
  historyBatch,
  historyEntries,
  historyQueueKey,
  historyRequest,
  historySnapshot,
  mergeHistory,
  mergeHistoryEntries,
} from "./history.js";

const track = (id, overrides = {}) => ({
  id: String(id),
  source: "music",
  title: `Track ${id}`,
  artist: `Artist ${id}`,
  ...overrides,
});

const record = (id, occurredAt, options = {}) =>
  createHistoryRecord(track(id, options.track), {
    id: () => options.idempotencyKey || `history-${id}-${occurredAt}`,
    now: () => occurredAt,
  });

const entry = (id, lastListenedAt, options = {}) => ({
  track: track(id, options.track),
  first_listened_at: options.first_listened_at || lastListenedAt,
  last_listened_at: lastListenedAt,
  play_count: options.play_count || 1,
});

test("creates a compact history record with the music adapter's canonical reference", () => {
  const created = createHistoryRecord(
    {
      id: "soundcloud:tracks:1534086151",
      source: " SoundCloud ",
      title: " Утро ",
      artist: " Дайте танк (!) ",
      artistId: "soundcloud:users:1030983220",
      description: "must not be queued",
    },
    {
      id: () => "history-1",
      now: () => "2026-10-03T12:34:56Z",
    },
  );

  assert.deepEqual(created, {
    idempotency_key: "history-1",
    occurred_at: "2026-10-03T12:34:56.000Z",
    generation: 0,
    track: {
      id: "1534086151",
      source: "soundcloud",
      title: "Утро",
      artist: "Дайте танк (!)",
      artistId: "1030983220",
    },
  });
  assert.equal(canonicalHistoryTrack({ id: "42", source: "music" }), null);
  assert.equal(
    createHistoryRecord(track("42"), {
      id: () => "",
      now: () => "2026-10-03T12:34:56Z",
    }),
    null,
  );
});

test("history queue retains repeat listens and acknowledges only the delivered record", () => {
  const first = record("42", "2026-10-03T10:00:00Z", {
    idempotencyKey: "history-first",
  });
  const repeated = record("42", "2026-10-03T10:01:00Z", {
    idempotencyKey: "history-repeat",
  });
  const nextTrack = record("99", "2026-10-03T10:02:00Z", {
    idempotencyKey: "history-next",
  });
  const queued = enqueueHistoryRecord(
    enqueueHistoryRecord([first], repeated),
    nextTrack,
  );

  assert.deepEqual(
    queued.map((item) => item.idempotency_key),
    ["history-first", "history-repeat", "history-next"],
  );
  assert.deepEqual(
    historyBatch(queued).map((item) => item.idempotency_key),
    ["history-first"],
  );
  assert.deepEqual(
    acknowledgeHistoryRecords(queued, [first]).map(
      (item) => item.idempotency_key,
    ),
    ["history-repeat", "history-next"],
  );

  const retried = enqueueHistoryRecord(queued, {
    ...first,
    occurred_at: "2026-10-03T10:03:00Z",
  });
  assert.deepEqual(
    retried.map((item) => item.idempotency_key),
    ["history-repeat", "history-next", "history-first"],
  );
});

test("request shape excludes queue bookkeeping and uses the original idempotent payload", () => {
  const value = {
    ...record("42", "2026-10-03T10:00:00Z", {
      idempotencyKey: "history-42",
    }),
    retry_after: "never sent to API",
  };
  assert.deepEqual(historyRequest(value), {
    idempotency_key: "history-42",
    occurred_at: "2026-10-03T10:00:00.000Z",
    generation: 0,
    track: track("42"),
  });
});

test("binds current-session listens to GET generation and drops pre-clear queue records", () => {
  const currentSession = createHistoryRecord(track("fresh"), {
    id: () => "history-fresh",
    now: () => "2026-10-03T10:00:00Z",
    generation: null,
  });
  const oldQueue = {
    idempotency_key: "history-old",
    occurred_at: "2026-10-03T09:00:00Z",
    track: track("old"),
    generation: 0,
  };
  const rebound = bindHistoryGeneration([oldQueue, currentSession], 1);
  assert.deepEqual(
    rebound.map((item) => [item.idempotency_key, item.generation]),
    [["history-fresh", 1]],
  );
  assert.equal(historyRequest(currentSession), null);
  assert.deepEqual(historySnapshot({ generation: 3, history: [] }), {
    generation: 3,
    entries: [],
  });
});

test("remote history is authoritative while unsent listens stay visible", () => {
  const remote = {
    history: [
      entry("remote-new", "2026-10-03T10:00:00Z", { play_count: 3 }),
      entry("remote-old", "2026-10-03T09:00:00Z"),
    ],
  };
  const pending = [record("offline", "2026-10-03T11:00:00Z")];
  const merged = mergeHistory(
    {
      likes: [track("liked")],
      history: [track("stale-browser-only")],
    },
    remote,
    pending,
  );

  assert.deepEqual(
    merged.history.map((item) => item.id),
    ["offline", "remote-new", "remote-old"],
  );
  assert.deepEqual(
    merged.likes.map((item) => item.id),
    ["liked"],
  );
});

test("server entries and an in-flight PUT reconcile without duplicate tracks", () => {
  const staleGet = [entry("42", "2026-10-03T10:00:00Z", { play_count: 2 })];
  const putResult = entry("42", "2026-10-03T10:05:00Z", {
    first_listened_at: "2026-10-03T09:00:00Z",
    play_count: 3,
  });
  const other = entry("7", "2026-10-03T10:03:00Z");
  const reconciled = mergeHistoryEntries(staleGet, [putResult, other]);

  assert.deepEqual(
    reconciled.map((item) => [item.track.id, item.play_count]),
    [
      ["42", 3],
      ["7", 1],
    ],
  );
  assert.equal(reconciled[0].first_listened_at, "2026-10-03T09:00:00.000Z");
  assert.equal(reconciled[0].last_listened_at, "2026-10-03T10:05:00.000Z");
});

test("history responses are normalized", () => {
  const entries = historyEntries({
    history: [
      entry("soundcloud:tracks:42", "2026-10-03T10:00:00Z", {
        track: { source: "soundcloud", artist: "Artist 42" },
      }),
      { track: track("invalid"), last_listened_at: "bad", play_count: 1 },
    ],
  });
  assert.equal(entries.length, 1);
  assert.equal(entries[0].track.id, "42");

  assert.equal(historyQueueKey("user-1"), "mixora-ui:history-queue:user-1");
});
