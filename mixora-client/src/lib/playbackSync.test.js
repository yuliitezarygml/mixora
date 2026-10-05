import test from "node:test";
import assert from "node:assert/strict";
import { playbackSnapshot, playerStorageSnapshot } from "./playbackSync.js";

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

test("long queues keep the active track in desktop sync and local restore", () => {
  const queue = Array.from({ length: 40 }, (_, index) => ({
    id: String(index),
    source: "soundcloud",
    title: `Трек ${index}`,
    artist: "Артист",
  }));
  const current = queue[35];
  const remote = playbackSnapshot(current, true, 3, queue);
  const local = playerStorageSnapshot(current, 35, 3, queue);

  assert.equal(remote.queue.length, 30);
  assert.equal(remote.queue[25].id, "35");
  assert.equal(local.queue.length, 30);
  assert.equal(local.index, 25);
  assert.equal(local.queue[local.index].id, "35");
});

test("external playback snapshots retain only the durable source page", () => {
  const track = {
    id: "bc-1",
    source: "bandcamp",
    title: "Saved track",
    artist: "Artist",
    permalink: "https://artist.bandcamp.com/track/saved-track",
    audio_url: "https://temporary.example/audio",
    secret: "never persist",
  };
  const remote = playbackSnapshot(track, true, 0, [track]);
  const local = playerStorageSnapshot(track, 0, 0, [track]);

  for (const snapshot of [
    remote.track,
    remote.queue[0],
    local.track,
    local.queue[0],
  ]) {
    assert.equal(snapshot.permalink, track.permalink);
    assert.equal(snapshot.audio_url, undefined);
    assert.equal(snapshot.secret, undefined);
  }
});
