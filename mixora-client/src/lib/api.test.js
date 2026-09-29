import test from "node:test";
import assert from "node:assert/strict";
import {
  api,
  catalogResource,
  getTrackPlayback,
  normalizeTrackPlayback,
  searchCatalog,
  soundcloudPlaylist,
  soundcloudResourceId,
  soundcloudTrack,
  unwrapApiResponse,
} from "./api.js";
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

test("unwraps the backend Response envelope while preserving plain payloads", () => {
  assert.deepEqual(unwrapApiResponse({ success: true, data: { id: 7 } }), {
    id: 7,
  });
  assert.deepEqual(unwrapApiResponse({ collection: [1, 2] }), {
    collection: [1, 2],
  });
  assert.throws(
    () => unwrapApiResponse({ success: false, error: "not playable" }, 404),
    (error) => error.status === 404 && error.message === "not playable",
  );
  assert.throws(
    () =>
      unwrapApiResponse(
        {
          success: false,
          error: {
            code: "session_expired",
            message: "Сессия истекла",
            request_id: "req-42",
          },
        },
        401,
      ),
    (error) =>
      error.message === "Сессия истекла" &&
      error.code === "session_expired" &&
      error.requestId === "req-42",
  );
});

test("api reads a structured application error", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () =>
    new Response(
      JSON.stringify({
        error: {
          code: "invalid_email",
          message: "Некорректный адрес почты",
          request_id: "req-7",
        },
      }),
      { status: 401, headers: { "Content-Type": "application/json" } },
    );
  try {
    await assert.rejects(
      () => api("/auth/register"),
      (error) =>
        error.status === 401 &&
        error.message === "Некорректный адрес почты" &&
        error.code === "invalid_email" &&
        error.requestId === "req-7",
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("extracts the numeric backend id from ids and SoundCloud URNs", () => {
  assert.equal(soundcloudResourceId(42), "42");
  assert.equal(
    soundcloudResourceId("soundcloud:tracks:1534086151"),
    "1534086151",
  );
  assert.equal(
    soundcloudResourceId("soundcloud%3Ausers%3A1030983220"),
    "1030983220",
  );
  assert.equal(
    soundcloudResourceId("https://api.soundcloud.com/playlists/987?secret=x"),
    "987",
  );
  assert.throws(() => soundcloudResourceId("not-an-id"), /идентификатор/);
});

test("normalizes both progressive and HLS playback responses", () => {
  assert.deepEqual(
    normalizeTrackPlayback({
      stream_url: "https://cdn.example/track.mp3",
      format: "progressive",
    }),
    {
      stream_url: "https://cdn.example/track.mp3",
      url: "https://cdn.example/track.mp3",
      format: "progressive",
    },
  );
  assert.equal(
    normalizeTrackPlayback({ url: "https://cdn.example/master.m3u8?token=x" })
      .format,
    "hls",
  );
});

test("catalog and playback call the ready backend routes", async () => {
  const originalFetch = globalThis.fetch;
  const calls = [];
  globalThis.fetch = async (url) => {
    calls.push(url);
    let data;
    if (url.includes("/search?")) {
      data = {
        collection: [{ id: 9, title: "Test", user: { username: "Artist" } }],
      };
    } else if (url.includes("/users/")) {
      data = { id: 77, username: "Artist" };
    } else {
      data = {
        stream_url: "https://cdn.example/track.mp3",
        format: "progressive",
      };
    }
    return new Response(
      JSON.stringify({
        success: true,
        data,
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    );
  };
  try {
    const tracks = await searchCatalog("tracks", "test query");
    assert.equal(tracks[0].title, "Test");
    await catalogResource("users", "soundcloud:users:77");
    const playback = await getTrackPlayback("soundcloud:tracks:123");
    assert.equal(playback.url, "https://cdn.example/track.mp3");
    assert.equal(
      calls[0],
      "/api/v1/search?q=test%20query&type=tracks&limit=40",
    );
    assert.equal(calls[1], "/api/v1/users/77");
    assert.equal(calls[2], "/api/v1/tracks/123/stream");
  } finally {
    globalThis.fetch = originalFetch;
  }
});
