import test from "node:test";
import assert from "node:assert/strict";
import { playbackSnapshot } from "./playbackSync.js";

test("playback snapshot keeps one state and a short queue", () => {
  const queue = Array.from({ length: 40 }, (_, i) => ({
    id: String(i),
    source: "soundcloud",
    title: "Трек",
    secret: "nope",
  }));
  const snap = playbackSnapshot(queue[0], true, 12.26, queue);
  assert.equal(snap.type, "state");
  assert.equal(snap.playing, true);
  assert.equal(snap.position, 12.3);
  assert.equal(snap.queue.length, 30);
  assert.equal(snap.track.secret, undefined);
  assert.equal(snap.queue[0].title, "Трек");
});
