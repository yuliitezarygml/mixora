import test from "node:test";
import assert from "node:assert/strict";
import {
  ApiError,
  api,
  catalogResource,
  getTrackPlayback,
  normalizeTrackPlayback,
  canonicalTrackReference,
  externalTrack,
  searchCatalog,
  searchSpotifyCatalog,
  searchYouTubeCatalog,
  resolveExternalTrack,
  presentRequestFailure,
  spotifyTrack,
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
  assert.equal(track.id, "1");
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
  assert.equal(playlist.tracks[1].id, "7");
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

test("api normalizes network failures without treating aborts as errors", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => {
    throw new TypeError("Failed to fetch");
  };
  try {
    await assert.rejects(
      () => api("/search?q=test"),
      (error) =>
        error instanceof ApiError &&
        error.status === 0 &&
        error.code === "network_unavailable" &&
        /сервером/.test(error.message),
    );
  } finally {
    globalThis.fetch = originalFetch;
  }

  const aborted = new Error("Request aborted");
  aborted.name = "AbortError";
  globalThis.fetch = async () => {
    throw aborted;
  };
  try {
    await assert.rejects(
      () => api("/search?q=test"),
      (error) => error === aborted,
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("api hides upstream diagnostics and error presentation stays actionable", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () =>
    new Response(
      JSON.stringify({
        error: "ERROR: provider returned a temporary signed media URL",
      }),
      { status: 502, headers: { "Content-Type": "application/json" } },
    );
  try {
    await assert.rejects(
      () => api("/extract?url=https%3A%2F%2Fexample.test"),
      (error) =>
        error.status === 502 &&
        /Источник временно/.test(error.message) &&
        !/SoundCloud|signed media/i.test(error.message),
    );
  } finally {
    globalThis.fetch = originalFetch;
  }

  const offline = presentRequestFailure(
    new ApiError("raw browser failure", 0, { code: "network_unavailable" }),
    { source: "spotify", online: false },
  );
  assert.deepEqual(
    { kind: offline.kind, title: offline.title, retryable: offline.retryable },
    {
      kind: "offline",
      title: "Нет подключения к интернету",
      retryable: true,
    },
  );

  const signIn = presentRequestFailure(new ApiError("session expired", 401), {
    source: "youtube",
  });
  assert.equal(signIn.action, "sign-in");
  assert.equal(signIn.retryable, false);

  const throttled = presentRequestFailure(new ApiError("", 429), {
    source: "bandcamp",
  });
  assert.equal(throttled.kind, "rate-limit");
  assert.equal(throttled.retryable, true);

  const unavailable = presentRequestFailure(new ApiError("", 503), {
    source: "vk",
  });
  assert.match(unavailable.title, /VK временно недоступен/);
  assert.equal(unavailable.retryable, true);

  const invalidLink = presentRequestFailure(
    new ApiError("Вставьте полную ссылку на трек или видео.", 400),
    { source: "external" },
  );
  assert.equal(invalidLink.text, "Вставьте полную ссылку на трек или видео.");
  assert.equal(invalidLink.retryable, false);
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

test("exposes canonical track references to state stores without leaking provider grammar", () => {
  assert.deepEqual(
    canonicalTrackReference({
      source: " SoundCloud ",
      id: "soundcloud:tracks:1534086151",
      artistId: "soundcloud:users:1030983220",
    }),
    {
      source: "soundcloud",
      id: "1534086151",
      artistId: "1030983220",
    },
  );
  assert.deepEqual(
    canonicalTrackReference({ source: "local", id: " local:42 " }),
    { source: "local", id: "local:42" },
  );
  assert.equal(
    canonicalTrackReference({ source: "soundcloud", id: "not-an-id" }),
    null,
  );
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

test("adapts Spotify and universal extractor results into provider-neutral tracks", () => {
  assert.deepEqual(
    spotifyTrack({
      id: "sp-1",
      title: "  Тест  ",
      artists: [{ id: "artist-1", name: "Исполнитель" }],
      duration_ms: 123000,
      preview_url: "https://cdn.example/preview.mp3",
      album: { name: "Альбом", images: [{ url: "https://img.example/a.jpg" }] },
      external_url: "https://open.spotify.com/track/sp-1",
    }),
    {
      id: "sp-1",
      source: "spotify",
      title: "Тест",
      artist: "Исполнитель",
      artistId: "artist-1",
      artwork: "https://img.example/a.jpg",
      duration: 123,
      explicit: false,
      access: "preview",
      permalink: "https://open.spotify.com/track/sp-1",
      album: "Альбом",
    },
  );
  assert.equal(
    spotifyTrack({ id: "connect-only", title: "Без превью" }).access,
    "blocked",
  );
  assert.deepEqual(
    externalTrack({
      id: "yt-1",
      title: "Видео",
      uploader: "Канал",
      duration: 42,
      extractor: "youtube",
      webpage_url: "https://www.youtube.com/watch?v=yt-1",
    }),
    {
      id: "yt-1",
      source: "youtube",
      title: "Видео",
      artist: "Канал",
      artwork: "",
      duration: 42,
      explicit: false,
      access: "playable",
      permalink: "https://www.youtube.com/watch?v=yt-1",
      album: "",
      description: "",
    },
  );
  assert.equal(
    externalTrack({
      id: "vk-1",
      title: "VK track",
      extractor: "generic",
      webpage_url: "https://vk.com/audio-1",
    }).source,
    "vk",
  );
});

test("uses provider-specific routes for Spotify, YouTube and universal links", async () => {
  const originalFetch = globalThis.fetch;
  const calls = [];
  globalThis.fetch = async (url) => {
    calls.push(url);
    let data;
    if (url.includes("/spotify/search")) {
      data = {
        tracks: [
          {
            id: "sp-1",
            title: "Spotify track",
            artists: [{ id: "artist-1", name: "Spotify artist" }],
            preview_url: "https://cdn.example/preview.mp3",
          },
        ],
      };
    } else if (url.includes("/youtube/search")) {
      data = {
        items: [
          {
            id: "yt-1",
            title: "YouTube track",
            uploader: "YouTube artist",
            // Flat yt-dlp search results may omit the extractor name.
            extractor: "",
            webpage_url: "https://www.youtube.com/watch?v=yt-1",
          },
        ],
      };
    } else if (url.includes("/spotify/tracks/")) {
      data = { preview_url: "https://cdn.example/preview.mp3" };
    } else {
      data = {
        id: "bc-1",
        title: "Bandcamp track",
        uploader: "Bandcamp artist",
        extractor: "bandcamp",
        webpage_url: "https://artist.bandcamp.com/track/test",
        audio_url: "https://cdn.example/track.m4a",
      };
    }
    return new Response(JSON.stringify({ success: true, data }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };
  try {
    const spotify = await searchSpotifyCatalog("tracks", "test");
    const youtube = await searchYouTubeCatalog("test");
    const bandcamp = await resolveExternalTrack(
      "https://artist.bandcamp.com/track/test",
    );
    const spotifyPlayback = await getTrackPlayback(spotify[0]);
    const externalPlayback = await getTrackPlayback(bandcamp);

    assert.equal(spotify[0].source, "spotify");
    assert.equal(youtube[0].source, "youtube");
    assert.equal(bandcamp.source, "bandcamp");
    assert.equal(spotifyPlayback.url, "https://cdn.example/preview.mp3");
    assert.equal(externalPlayback.url, "https://cdn.example/track.m4a");
    assert.deepEqual(calls, [
      "/api/v1/spotify/search?q=test&type=track&limit=40",
      "/api/v1/youtube/search?q=test&limit=25",
      "/api/v1/extract?url=https%3A%2F%2Fartist.bandcamp.com%2Ftrack%2Ftest",
      "/api/v1/spotify/tracks/sp-1/stream",
      "/api/v1/extract?url=https%3A%2F%2Fartist.bandcamp.com%2Ftrack%2Ftest",
    ]);
  } finally {
    globalThis.fetch = originalFetch;
  }
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
