import test from "node:test";
import assert from "node:assert/strict";
import { buildWaveRequest, decodeWaveResponse } from "./waveRequest.js";

const track = (id, source = "soundcloud") => ({
  id: String(id),
  source,
  title: `Track ${id}`,
});

test("buildWaveRequest keeps canonical seed order and applies transport limits", () => {
  const likes = Array.from({ length: 45 }, (_, index) => track(index + 1));
  const history = [track(1), ...Array.from({ length: 45 }, (_, index) => track(index + 46))];
  const dislikes = Array.from({ length: 85 }, (_, index) => track(index + 100));
  const catalog = [track(2), track("local", "local")];

  const result = buildWaveRequest({
    library: { likes, history, dislikes },
    catalog,
    preferences: { mood: "calm" },
    context: { title: "focus" },
    round: 7,
    explicit: false,
    exclude: [track("blocked")],
  });

  assert.deepEqual(result.preferences, { mood: "calm" });
  assert.deepEqual(result.context, { title: "focus" });
  assert.equal(result.round, 7);
  assert.equal(result.explicit, false);
  assert.deepEqual(result.exclude, [track("blocked")]);
  assert.equal(result.likes.length, 40);
  assert.equal(result.history.length, 40);
  assert.equal(result.dislikes.length, 80);
  assert.equal(result.seeds.length, 40);
  assert.deepEqual(
    result.seeds.map((item) => item.id),
    Array.from({ length: 40 }, (_, index) => String(index + 1)),
  );
});

test("buildWaveRequest turns missing context into an empty object", () => {
  const result = buildWaveRequest({
    library: {},
    catalog: [],
    preferences: {},
    round: 0,
    explicit: true,
  });

  assert.deepEqual(result.context, {});
  assert.deepEqual(result.exclude, []);
  assert.deepEqual(result.seeds, []);
});

test("decodeWaveResponse ignores a session when the response has no tracks", () => {
  assert.deepEqual(
    decodeWaveResponse({ session_id: "session-1", model_version: "content-v2" }),
    { tracks: [], sessionId: "", modelVersion: "content-v2" },
  );
});

test("decodeWaveResponse defaults the missing model version", () => {
  const tracks = [track(42)];
  assert.deepEqual(decodeWaveResponse({ tracks, session_id: "session-1" }), {
    tracks,
    sessionId: "session-1",
    modelVersion: "rules-v0",
  });
});
