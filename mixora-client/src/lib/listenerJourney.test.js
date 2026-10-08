import assert from "node:assert/strict";
import test from "node:test";
import { findKnownTrack } from "./library.js";
import { trackListeningEvent } from "./listeningEvents.js";
import {
  createPlaylistMutation,
  mergePlaylists,
  playlistState,
} from "./playlists.js";
import { playbackSnapshot, playerStorageSnapshot } from "./playbackSync.js";
import {
  createTrackPreferenceMutation,
  mergeTrackPreferences,
  trackPreferenceState,
} from "./trackPreferences.js";

const playlistID = "123e4567-e89b-12d3-a456-426614174100";

const tracks = [
  {
    id: "soundcloud:tracks:42",
    source: "soundcloud",
    title: "SoundCloud track",
    artist: "Artist",
    permalink: "https://soundcloud.com/artist/track",
  },
  {
    id: "youtube-1",
    source: "youtube",
    title: "YouTube track",
    artist: "Artist",
    permalink: "https://www.youtube.com/watch?v=youtube-1",
    audio_url: "https://temporary.invalid/youtube",
  },
  {
    id: "vk-1",
    source: "vk",
    title: "VK track",
    artist: "Artist",
    permalink: "https://vkvideo.ru/video1_2",
  },
  {
    id: "bandcamp-1",
    source: "bandcamp",
    title: "Bandcamp track",
    artist: "Artist",
    permalink: "https://artist.bandcamp.com/track/example",
  },
  {
    id: "spotify-1",
    source: "spotify",
    title: "Spotify preview",
    artist: "Artist",
    permalink: "https://open.spotify.com/track/spotify-1",
    access: "preview",
  },
];

const emptyLibrary = () => ({
  likes: [],
  dislikes: [],
  history: [],
  playlists: [],
  savedPlaylists: [],
  pins: [],
});

test("play, like, next and playlist state survive a provider-aware reopen", () => {
  // The listener starts the first result and advances to YouTube. Persisting
  // that state must restore the same item and retain every provider page that
  // is required to resolve a fresh stream after an app restart.
  const savedPlayer = playerStorageSnapshot(tracks[1], 1, 17.26, tracks);
  const restoredTrack = savedPlayer.queue[savedPlayer.index];
  assert.equal(restoredTrack.source, "youtube");
  assert.equal(restoredTrack.id, "youtube-1");
  assert.equal(savedPlayer.position, 17.3);
  assert.deepEqual(
    savedPlayer.queue.map((track) => track.source),
    ["soundcloud", "youtube", "vk", "bandcamp", "spotify"],
  );
  assert.equal(savedPlayer.queue[1].audio_url, undefined);
  assert.equal(
    playbackSnapshot(
      restoredTrack,
      false,
      savedPlayer.position,
      savedPlayer.queue,
    ).track.permalink,
    tracks[1].permalink,
  );

  const like = createTrackPreferenceMutation(restoredTrack, "liked", {
    id: () => "journey-like",
  });
  const optimisticLibrary = mergeTrackPreferences(emptyLibrary(), [], [like], {
    authoritative: false,
  });
  assert.equal(optimisticLibrary.likes[0].id, "youtube-1");

  const remotePreference = trackPreferenceState([
    { track: like.track, preference: "liked", revision: 1 },
  ]);
  const reopenedLibrary = mergeTrackPreferences(
    emptyLibrary(),
    remotePreference,
  );
  assert.deepEqual(
    reopenedLibrary.likes.map((track) => `${track.source}:${track.id}`),
    ["youtube:youtube-1"],
  );

  const playlistMutation = createPlaylistMutation(
    {
      id: playlistID,
      name: "Journey mix",
      tracks: savedPlayer.queue,
      pinned: true,
    },
    { id: () => "journey-playlist" },
  );
  const optimisticPlaylistLibrary = mergePlaylists(
    reopenedLibrary,
    [],
    [playlistMutation],
    { authoritative: false },
  );
  assert.equal(optimisticPlaylistLibrary.playlists[0].tracks.length, 5);

  const [serverPlaylist] = playlistState([
    { ...playlistMutation.playlist, revision: 1 },
  ]);
  const reopenedWithPlaylist = mergePlaylists(reopenedLibrary, [
    serverPlaylist,
  ]);
  assert.deepEqual(
    reopenedWithPlaylist.playlists[0].tracks.map((track) => track.source),
    ["soundcloud", "youtube", "vk", "bandcamp", "spotify"],
  );
  assert.equal(reopenedWithPlaylist.playlists[0].tracks[0].id, "42");
  assert.equal(
    findKnownTrack([], reopenedWithPlaylist, "bandcamp", "bandcamp-1")
      ?.permalink,
    tracks[3].permalink,
  );

  const playEvent = trackListeningEvent(
    "play",
    restoredTrack,
    {},
    {
      id: () => "journey-play",
      now: () => "2026-10-05T08:00:00.000Z",
    },
  );
  const nextEvent = trackListeningEvent(
    "skip",
    restoredTrack,
    {},
    {
      id: () => "journey-next",
      now: () => "2026-10-05T08:00:01.000Z",
    },
  );
  assert.deepEqual(
    [playEvent, nextEvent].map((event) => [event.type, event.track_source]),
    [
      ["play", "youtube"],
      ["skip", "youtube"],
    ],
  );
});
