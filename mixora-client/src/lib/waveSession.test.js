import test from "node:test";
import assert from "node:assert/strict";
import {
  createWaveSessionSnapshot,
  restoreWaveSession,
  updateWaveTrackSessions,
  waveSessionStorageKey,
} from "./waveSession.js";

const firstSession = "123e4567-e89b-42d3-a456-426614174000";
const secondSession = "123e4567-e89b-42d3-a456-426614174001";
const track = (id) => ({ source: "soundcloud", id: String(id) });

test("wave resume snapshot keeps only compact session ownership", () => {
  const sessions = updateWaveTrackSessions(
    new Map(),
    [track(1), track(2)],
    firstSession,
  );
  const snapshot = createWaveSessionSnapshot({
    modelVersion: "gorse-embeddinggemma-v1",
    preferences: {
      activity: "road",
      diversity: "unknown",
      mood: "calm",
      language: "foreign",
    },
    context: { artist: " Artist\u0000 ", ignored: "must not persist" },
    round: 4,
    trackSessions: sessions,
  });

  assert.equal(snapshot.version, 1);
  assert.equal(snapshot.model_version, "gorse-embeddinggemma-v1");
  assert.deepEqual(snapshot.context, { artist: "Artist" });
  assert.deepEqual(snapshot.preferences, {
    activity: "road",
    diversity: "unknown",
    mood: "calm",
    language: "foreign",
  });
  assert.equal(snapshot.round, 4);
  assert.deepEqual(snapshot.track_sessions, [
    ["soundcloud:1", firstSession],
    ["soundcloud:2", firstSession],
  ]);
  assert.equal(
    waveSessionStorageKey("account-1"),
    "mixora-ui:wave-session-v1:account-1",
  );
});

test("restoring a wave session binds only tracks that came back in the player queue", () => {
  const snapshot = createWaveSessionSnapshot({
    modelVersion: "gorse-v1",
    preferences: { mood: "happy" },
    context: { genre: "indie" },
    round: 7,
    trackSessions: [
      ["soundcloud:1", firstSession],
      ["soundcloud:2", secondSession],
      ["bad-key", "not-a-session"],
    ],
  });
  const restored = restoreWaveSession(snapshot, [track(2), track(3)]);

  assert.deepEqual(restored, {
    modelVersion: "gorse-v1",
    preferences: {
      activity: "any",
      diversity: "any",
      mood: "happy",
      language: "any",
    },
    context: { genre: "indie" },
    round: 7,
    trackSessions: new Map([["soundcloud:2", secondSession]]),
  });
  assert.equal(restoreWaveSession({ version: 2 }, [track(1)]), null);
  assert.equal(restoreWaveSession(snapshot, []), null);

  const malformedPreferences = restoreWaveSession(
    { version: 1, preferences: null, track_sessions: [] },
    [track(1)],
  );
  assert.deepEqual(malformedPreferences.preferences, {
    activity: "any",
    diversity: "any",
    mood: "any",
    language: "any",
  });
});

test("local fallback tracks do not inherit an old server session", () => {
  const first = updateWaveTrackSessions(
    new Map(),
    [track(1), track(2)],
    firstSession,
  );
  const second = updateWaveTrackSessions(first, [track(1)], "");

  assert.deepEqual([...second.entries()], [["soundcloud:2", firstSession]]);
});
