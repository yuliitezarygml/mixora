import test from "node:test";
import assert from "node:assert/strict";
import { soundcloudPlaylist, soundcloudTrack } from "./api.js";
test("uses artist metadata instead of uploader, preserving original Unicode titles", () => {
  const track = soundcloudTrack({
    urn: "soundcloud:tracks:1",
    title: "  Бейба судьба  ",
    metadata_artist: "Miyagi & Эндшпиль",
    user: { username: "uploader" },
    duration: 1000,
  });
  assert.equal(track.title, "Бейба судьба");
  assert.equal(track.artist, "Miyagi & Эндшпиль");
  assert.equal(track.duration, 1);
});
test("falls back from empty artist metadata without destroying names in other languages", () => {
  const track = soundcloudTrack({
    id: 2,
    title: "Nếu Ngày Ấy",
    metadata_artist: " ",
    user: { username: "Thanh Định" },
  });
  assert.equal(track.artist, "Thanh Định");
  assert.equal(track.title, "Nếu Ngày Ấy");
});
test("playlist keeps tracks that arrive with only an id", () => {
  const playlist = soundcloudPlaylist({
    urn: "soundcloud:playlists:9",
    title: "Другой ритм",
    tracks: [{ id: 42 }, { urn: "soundcloud:tracks:7", title: "Утро" }],
  });
  assert.equal(playlist.tracks.length, 2);
  assert.equal(playlist.tracks[0].id, "42");
  assert.equal(playlist.tracks[0].title, "Без названия");
  assert.equal(playlist.tracks[1].title, "Утро");
});
