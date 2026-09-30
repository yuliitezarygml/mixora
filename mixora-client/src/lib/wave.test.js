import test from "node:test";
import assert from "node:assert/strict";
import { buildWave, defaultWave, waveExplanation, waveQuery } from "./wave.js";
const track = (id, patch = {}) => ({
  id,
  source: "soundcloud",
  title: id,
  artist: "Artist",
  genre: "electronic",
  access: "playable",
  ...patch,
});
test("wave excludes blocked, disliked, queued and explicit tracks when disabled", () => {
  const tracks = [
    track("a"),
    track("b", { access: "blocked" }),
    track("c"),
    track("d", { explicit: true }),
    track("e"),
  ];
  const result = buildWave(
    tracks,
    { dislikes: [tracks[2]] },
    { ...defaultWave },
    { explicit: false, exclude: [tracks[0]], random: () => 0.5 },
  );
  assert.deepEqual(
    result.map((t) => t.id),
    ["e"],
  );
});
test("favorite wave keeps likes and unknown wave leaves them out", () => {
  const known = track("known");
  const fresh = track("new");
  const library = { likes: [known], history: [] };
  assert.deepEqual(
    buildWave([known, fresh], library, {
      ...defaultWave,
      diversity: "favorite",
    }).map((t) => t.id),
    ["known"],
  );
  assert.deepEqual(
    buildWave([known, fresh], library, {
      ...defaultWave,
      diversity: "unknown",
    }).map((t) => t.id),
    ["new"],
  );
});
test("popular wave prefers often played tracks and instrumental keeps wordless ones", () => {
  const quiet = track("quiet", {
    title: "Piano",
    genre: "instrumental",
    playbackCount: 10,
  });
  const loud = track("loud", {
    title: "Vocal",
    genre: "pop",
    playbackCount: 1000,
  });
  assert.deepEqual(
    buildWave(
      [quiet, loud],
      {},
      { ...defaultWave, diversity: "popular" },
      {
        random: () => 0,
      },
    ).map((t) => t.id),
    ["loud", "quiet"],
  );
  assert.deepEqual(
    buildWave(
      [quiet, loud],
      {},
      { ...defaultWave, language: "instrumental" },
    ).map((t) => t.id),
    ["quiet"],
  );
});
test("wave deduplicates providers and respects a selected language", () => {
  const ru = track("ru", { title: "Утро", artist: "Дайте танк" });
  const en = track("en", { title: "Awake", artist: "Tycho" });
  assert.deepEqual(
    buildWave([ru, ru, en], {}, { ...defaultWave, language: "russian" }).map(
      (t) => t.id,
    ),
    ["ru"],
  );
});
test("wave seed follows context and mood instead of a fixed catalogue offset", () => {
  assert.match(waveQuery({ ...defaultWave, mood: "calm" }, {}), /ambient/);
  assert.match(waveQuery({ ...defaultWave, activity: "wake" }, {}), /morning/);
  assert.match(
    waveQuery({ ...defaultWave, language: "instrumental" }, {}),
    /instrumental/,
  );
  assert.equal(waveQuery(defaultWave, {}, { artist: "Tycho" }), "Tycho");
});

test("wave explains whether personalization or rules produced the queue", () => {
  assert.match(waveExplanation("gorse-v1+rules-v0"), /истории/);
  assert.match(waveExplanation("rules-v0"), /настроению/);
});
