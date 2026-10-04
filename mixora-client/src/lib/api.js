export class ApiError extends Error {
  constructor(message, status, details = null) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = details?.code || "";
    this.requestId = details?.request_id || "";
  }
}

const errorDetails = (value) =>
  value !== null && typeof value === "object" ? value : null;

const errorMessage = (value) =>
  typeof value === "string"
    ? value
    : typeof value?.message === "string"
      ? value.message
      : "";

const isResponseEnvelope = (value) =>
  value !== null &&
  typeof value === "object" &&
  !Array.isArray(value) &&
  typeof value.success === "boolean" &&
  (Object.hasOwn(value, "data") || Object.hasOwn(value, "error"));

export function unwrapApiResponse(value, status = 200) {
  if (!isResponseEnvelope(value)) return value;
  if (!value.success) {
    throw new ApiError(
      errorMessage(value.error) || "Не удалось выполнить запрос.",
      status,
      errorDetails(value.error),
    );
  }
  return value.data;
}

export async function api(path, { signal, ...options } = {}) {
  const response = await fetch(`/api/v1${path}`, {
    credentials: "include",
    signal,
    ...options,
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...options.headers,
    },
  });
  if (response.status === 204) return null;
  let data;
  try {
    data = await response.json();
  } catch {
    throw new ApiError(
      "Сервер временно недоступен. Попробуйте ещё раз.",
      response.status,
    );
  }
  if (!response.ok) {
    const messages = {
      401: "Войдите в аккаунт, чтобы продолжить.",
      403: "Этот трек недоступен для воспроизведения.",
      409: "Этот адрес уже зарегистрирован.",
      429: "Слишком много запросов. Попробуйте через минуту.",
      502: "Не удалось получить аудио от SoundCloud. Попробуйте ещё раз.",
      503: "Музыкальный сервис временно недоступен.",
    };
    const details = errorDetails(data?.error);
    throw new ApiError(
      (details && errorMessage(details)) ||
        messages[response.status] ||
        errorMessage(data?.error) ||
        "Не удалось выполнить запрос.",
      response.status,
      details,
    );
  }
  return unwrapApiResponse(data, response.status);
}
export const post = (path, body) =>
  api(path, {
    method: "POST",
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
export const put = (path, body) =>
  api(path, { method: "PUT", body: JSON.stringify(body) });

export function soundcloudResourceId(value) {
  let raw = String(value ?? "").trim();
  try {
    raw = decodeURIComponent(raw);
  } catch {
    // Keep the original value. It can still be a valid plain numeric id.
  }
  if (/^\d+$/.test(raw)) return raw;

  const withoutQuery = raw.split(/[?#]/, 1)[0];
  const urn = withoutQuery.match(/(?:^|:)(\d+)$/);
  if (urn) return urn[1];
  const apiPath = withoutQuery.match(
    /\/(?:tracks|users|playlists)\/(\d+)\/?$/i,
  );
  if (apiPath) return apiPath[1];

  throw new ApiError("Некорректный идентификатор ресурса SoundCloud.", 400);
}

// Keep provider-specific identifier grammar at the music API boundary. State
// stores and UI components use this compact reference instead of knowing how
// a particular catalog writes its IDs (for example a SoundCloud URN).
export function canonicalTrackReference(track) {
  if (!track || typeof track !== "object") return null;
  const source = String(track.source ?? "")
    .trim()
    .toLowerCase();
  const rawID = String(track.id ?? "").trim();
  if (!source || !rawID) return null;

  try {
    const id = source === "soundcloud" ? soundcloudResourceId(rawID) : rawID;
    const rawArtistID = String(track.artistId ?? "").trim();
    const artistId = rawArtistID
      ? source === "soundcloud"
        ? soundcloudResourceId(rawArtistID)
        : rawArtistID
      : "";
    return { source, id, ...(artistId ? { artistId } : {}) };
  } catch {
    return null;
  }
}

const displayName = (value) =>
  typeof value === "string" ? value.trim().normalize("NFC") : "";

const firstText = (...values) => {
  for (const value of values) {
    const text = displayName(value);
    if (text) return text;
  }
  return "";
};

const nonNegativeNumber = (value) => {
  const number = Number(value);
  return Number.isFinite(number) && number >= 0 ? number : 0;
};

// These are the sources exposed by the running music engine. `external` is
// intentionally a URL resolver rather than a fourth catalogue: yt-dlp can
// resolve many providers (VK, Bandcamp and others), but only YouTube has a
// search endpoint that does not require an account or provider credentials.
export const musicSources = [
  {
    id: "soundcloud",
    label: "SoundCloud",
    kinds: ["all", "tracks", "artists", "albums", "playlists"],
  },
  {
    id: "spotify",
    label: "Spotify",
    kinds: ["all", "tracks", "artists", "albums", "playlists"],
  },
  { id: "youtube", label: "YouTube / YouTube Music", kinds: ["all", "tracks"] },
  {
    id: "external",
    label: "Ссылка: YouTube, VK, Bandcamp…",
    kinds: ["all", "tracks"],
  },
  { id: "local", label: "Mixora", kinds: ["all", "tracks"] },
];

export function sourceLabel(source) {
  const normalized = String(source ?? "")
    .trim()
    .toLowerCase();
  const labels = {
    soundcloud: "SoundCloud",
    spotify: "Spotify",
    youtube: "YouTube",
    "youtube-music": "YouTube Music",
    vk: "VK",
    vkontakte: "VK",
    bandcamp: "Bandcamp",
    local: "Mixora",
  };
  return labels[normalized] || displayName(source) || "Источник";
}

function externalSource(value, webpageURL = "") {
  let raw = String(value ?? "")
    .trim()
    .toLowerCase();
  if (raw.includes(":")) raw = raw.split(":", 1)[0];
  if (raw === "youtube-music") return "youtube-music";
  if (raw.startsWith("youtube")) return "youtube";
  if (raw.startsWith("vk") || raw.startsWith("vkontakte")) return "vk";
  if (raw.startsWith("bandcamp")) return "bandcamp";
  if (raw && raw !== "generic") {
    const compact = raw
      .replace(/[^a-z0-9._-]+/g, "-")
      .replace(/^-+|-+$/g, "");
    if (compact) return compact;
  }

  // Keep the browser's durable source namespace aligned with the backend
  // catalog observer when yt-dlp returns its generic extractor name.
  try {
    const host = new URL(webpageURL).hostname
      .toLowerCase()
      .replace(/^www\./, "");
    if (host === "youtu.be" || host === "youtube.com" || host.endsWith(".youtube.com")) {
      return "youtube";
    }
    if (host === "bandcamp.com" || host.endsWith(".bandcamp.com")) {
      return "bandcamp";
    }
    if (
      host === "vk.com" ||
      host.endsWith(".vk.com") ||
      host === "vkvideo.ru" ||
      host.endsWith(".vkvideo.ru")
    ) {
      return "vk";
    }
    const compact = host.replace(/[^a-z0-9._-]+/g, "-");
    if (compact) return compact;
  } catch {
    // The backend will return a clear resolver error for an invalid URL.
  }
  return "external";
}

function spotifyArtwork(value) {
  const images = Array.isArray(value?.images) ? value.images : [];
  return firstText(...images.map((image) => image?.url));
}

function spotifyArtists(value) {
  const artists = Array.isArray(value?.artists) ? value.artists : [];
  const names = artists
    .map((artist) => displayName(artist?.name))
    .filter(Boolean);
  const first = artists.find((artist) => displayName(artist?.name));
  return {
    artist: names.join(", ") || "Исполнитель",
    artistId: firstText(first?.id, first?.uri?.replace(/^spotify:artist:/, "")),
    artistUrl: firstText(first?.external_url),
  };
}

// Provider adapters produce the one compact track shape consumed by player,
// likes, history and playlists. Provider-specific response schemas do not leak
// beyond this module.
export function spotifyTrack(value) {
  const artists = spotifyArtists(value);
  return {
    id: String(value?.id ?? "").trim(),
    source: "spotify",
    title: firstText(value?.title, value?.name) || "Без названия",
    artist: artists.artist,
    ...(artists.artistId ? { artistId: artists.artistId } : {}),
    artwork: spotifyArtwork(value?.album) || spotifyArtwork(value),
    duration: nonNegativeNumber(value?.duration_ms) / 1000,
    explicit: value?.explicit === true,
    // `preview` makes the limited Spotify browser stream explicit in the UI.
    // If the backend cannot provide preview audio, playback will explain why
    // instead of pretending that a Connect-only source is browser-playable.
    // A browser player can legally use Spotify's preview URL, but not a full
    // Connect-only stream. Keep unavailable tracks visible as metadata while
    // making their non-playability explicit to every caller.
    access: value?.preview_url ? "preview" : "blocked",
    permalink: firstText(value?.external_url),
    ...(artists.artistUrl ? { artistUrl: artists.artistUrl } : {}),
    album: firstText(value?.album?.name),
  };
}

export function spotifyArtist(value) {
  return {
    id: String(value?.id ?? "").trim(),
    source: "spotify",
    name: firstText(value?.name, value?.title) || "Исполнитель",
    artwork: spotifyArtwork(value),
    permalink: firstText(value?.external_url),
    followers: nonNegativeNumber(value?.followers),
  };
}

export function spotifyPlaylist(value, { album = false } = {}) {
  const tracks = Array.isArray(value?.tracks) ? value.tracks : [];
  return {
    id: String(value?.id ?? "").trim(),
    source: "spotify",
    name: firstText(value?.name, value?.title) || "Без названия",
    artwork: spotifyArtwork(value) || spotifyArtwork(tracks[0]?.album),
    description: displayName(value?.description),
    artist: firstText(value?.owner, value?.artists?.[0]?.name),
    tracks: tracks.filter((track) => track?.id).map(spotifyTrack),
    count: nonNegativeNumber(value?.total_tracks) || tracks.length,
    album: album || value?.album_type === "album",
    permalink: firstText(value?.external_url),
  };
}

export function externalTrack(value, fallbackSource = "") {
  // yt-dlp's flat YouTube search intentionally omits `extractor` on some
  // versions. The caller knows which catalogue produced the result, so keep
  // that provenance instead of turning an ordinary YouTube result into the
  // vague `external` source.
  const source = externalSource(
    value?.extractor || fallbackSource,
    value?.webpage_url,
  );
  const id = firstText(value?.id, value?.webpage_url);
  return {
    id,
    source,
    title: firstText(value?.title) || "Без названия",
    artist: firstText(value?.artist, value?.uploader) || "Исполнитель",
    artwork: firstText(value?.thumbnail),
    duration: nonNegativeNumber(value?.duration),
    explicit: false,
    access: id && value?.webpage_url ? "playable" : "blocked",
    permalink: firstText(value?.webpage_url),
    album: firstText(value?.album),
    description: displayName(value?.description),
  };
}
export function soundcloudTrack(t) {
  return {
    id: soundcloudResourceId(t.id ?? t.urn),
    source: "soundcloud",
    title: displayName(t.title) || "Без названия",
    artist:
      displayName(t.metadata_artist) ||
      displayName(t.publisher_metadata?.artist) ||
      displayName(t.user?.username) ||
      "Исполнитель",
    artistId:
      t.user?.id != null || t.user?.urn
        ? soundcloudResourceId(t.user.id ?? t.user.urn)
        : "",
    artwork: t.artwork_url || t.user?.avatar_url || "",
    duration: (t.duration || 0) / 1000,
    explicit: t.explicit === true,
    access: t.access || "playable",
    permalink: t.permalink_url || "",
    artistUrl: t.user?.permalink_url || "",
    genre: t.genre || "",
    description: t.description || "",
    playbackCount: t.playback_count || 0,
    createdAt: t.created_at || "",
    album: t.publisher_metadata?.album_title || "",
    tagList: t.tag_list || "",
  };
}
export const collectionItems = (data) => {
  const value = unwrapApiResponse(data);
  return Array.isArray(value) ? value : value?.collection || [];
};
export function soundcloudArtist(data) {
  return {
    id: soundcloudResourceId(data.id ?? data.urn),
    source: "soundcloud",
    name: displayName(data.username),
    artwork: data.avatar_url || "",
    description: data.description || "",
    permalink: data.permalink_url || "",
    followers: data.followers_count || 0,
  };
}
export function soundcloudPlaylist(data) {
  return {
    id: soundcloudResourceId(data.id ?? data.urn),
    name: displayName(data.title),
    artwork: data.artwork_url || data.tracks?.[0]?.artwork_url || "",
    description: data.description || "",
    artist: displayName(data.user?.username),
    artistId:
      data.user?.id != null || data.user?.urn
        ? soundcloudResourceId(data.user.id ?? data.user.urn)
        : "",
    tracks: (data.tracks || [])
      .filter((t) => t && (t.title || t.urn || t.id))
      .map(soundcloudTrack),
    count: data.track_count || 0,
    album: data.is_album === true || data.playlist_type === "album",
    permalink: data.permalink_url || "",
    source: "soundcloud",
  };
}
export async function searchCatalog(kind, q, signal) {
  const data = await api(
    `/search?q=${encodeURIComponent(q)}&type=${encodeURIComponent(kind)}&limit=40`,
    { signal },
  );
  const convert =
    kind === "users"
      ? soundcloudArtist
      : kind === "playlists"
        ? soundcloudPlaylist
        : soundcloudTrack;
  return collectionItems(data).map(convert);
}

export async function searchSpotifyCatalog(kind, q, signal) {
  const types = {
    tracks: "track",
    artists: "artist",
    albums: "album",
    playlists: "playlist",
  };
  const type = types[kind];
  if (!type) return [];
  const data = await api(
    `/spotify/search?q=${encodeURIComponent(q)}&type=${type}&limit=40`,
    { signal },
  );
  const values =
    kind === "tracks"
      ? data?.tracks
      : kind === "artists"
        ? data?.artists
        : kind === "albums"
          ? data?.albums
          : data?.playlists;
  const convert =
    kind === "tracks"
      ? spotifyTrack
      : kind === "artists"
        ? spotifyArtist
        : (value) => spotifyPlaylist(value, { album: kind === "albums" });
  return (Array.isArray(values) ? values : [])
    .map(convert)
    .filter((value) => value.id);
}

export async function searchYouTubeCatalog(q, signal) {
  const data = await api(
    `/youtube/search?q=${encodeURIComponent(q)}&limit=25`,
    { signal },
  );
  return (Array.isArray(data?.items) ? data.items : [])
    .map((item) => externalTrack(item, "youtube"))
    .filter((value) => value.id && value.access !== "blocked");
}

export async function resolveExternalTrack(url, signal) {
  const target = String(url ?? "").trim();
  if (!/^https?:\/\//i.test(target)) {
    throw new ApiError("Вставьте полную ссылку на трек или видео.", 400);
  }
  const data = await api(`/extract?url=${encodeURIComponent(target)}`, {
    signal,
  });
  const track = externalTrack(data);
  if (!track.id || track.access === "blocked") {
    throw new ApiError("По этой ссылке не найден доступный аудиотрек.", 404);
  }
  return track;
}

export async function providerArtistTracks(artist, signal) {
  const source = String(artist?.source ?? "soundcloud").toLowerCase();
  if (source === "spotify") {
    const data = await api(
      `/spotify/artists/${encodeURIComponent(String(artist?.id ?? ""))}`,
      { signal },
    );
    return (Array.isArray(data?.top_tracks) ? data.top_tracks : [])
      .map(spotifyTrack)
      .filter((track) => track.id);
  }
  const data = await catalogResource("users", artist?.id, "tracks", signal);
  return collectionItems(data).map(soundcloudTrack);
}

export async function providerPlaylistTracks(playlist, signal) {
  const source = String(playlist?.source ?? "soundcloud").toLowerCase();
  if (source === "spotify") {
    const data = await api(
      `/spotify/playlists/${encodeURIComponent(String(playlist?.id ?? ""))}`,
      { signal },
    );
    return spotifyPlaylist(data).tracks;
  }
  const data = await catalogResource("playlists", playlist?.id, "", signal);
  return soundcloudPlaylist(data).tracks;
}
export function catalogResource(kind, id, section = "", signal) {
  const resourceId = soundcloudResourceId(id);
  if (kind === "users" && section === "playlists") {
    // The current backend exposes user profiles and tracks, but no user-playlists
    // route yet. Keep the details screen usable without calling a stale endpoint.
    return Promise.resolve({ collection: [] });
  }

  const routes = {
    tracks: `/tracks/${resourceId}`,
    users: `/users/${resourceId}${section === "tracks" ? "/tracks" : ""}`,
    playlists: `/playlists/${resourceId}`,
  };
  const route = routes[kind];
  if (!route) {
    return Promise.reject(
      new ApiError("Неизвестный тип ресурса каталога.", 400),
    );
  }
  return api(route, { signal });
}

export function normalizeTrackPlayback(data) {
  const value = unwrapApiResponse(data);
  const url =
    value?.stream_url ||
    value?.preview_url ||
    value?.audio_url ||
    value?.url ||
    "";
  if (!url) {
    throw new ApiError("Сервер не вернул ссылку на аудиопоток.", 502);
  }
  const rawFormat =
    typeof value.format === "string"
      ? value.format
      : value.format?.protocol || value.protocol || "";
  const format =
    rawFormat.toLowerCase() === "hls" || /\.m3u8(?:$|[?#])/i.test(url)
      ? "hls"
      : "progressive";
  return { ...value, url, format };
}

export async function getTrackPlayback(trackOrID, signal) {
  // A plain ID remains a SoundCloud-compatible public API for older callers,
  // while new player calls pass the full provider-neutral track.
  const track =
    trackOrID && typeof trackOrID === "object"
      ? trackOrID
      : { id: trackOrID, source: "soundcloud" };
  const reference = canonicalTrackReference(track);
  if (!reference) {
    throw new ApiError("Некорректная ссылка на трек.", 400);
  }
  if (reference.source === "soundcloud") {
    const data = await api(
      `/tracks/${encodeURIComponent(reference.id)}/stream`,
      { signal },
    );
    return normalizeTrackPlayback(data);
  }
  if (reference.source === "spotify") {
    const data = await api(
      `/spotify/tracks/${encodeURIComponent(reference.id)}/stream`,
      { signal },
    );
    if (!data?.preview_url) {
      throw new ApiError(
        data?.connect_available
          ? "Этот трек Spotify доступен только через Spotify Connect, без браузерного превью."
          : "Spotify не предоставил браузерное превью для этого трека.",
        422,
      );
    }
    return normalizeTrackPlayback(data);
  }
  const permalink = firstText(track?.permalink, track?.webpage_url);
  if (!permalink) {
    throw new ApiError(
      "Для этого источника не сохранена исходная ссылка.",
      400,
    );
  }
  const data = await api(`/extract?url=${encodeURIComponent(permalink)}`, {
    signal,
  });
  return normalizeTrackPlayback(data);
}
export function localTrack(t) {
  return {
    ...t,
    source: "local",
    artwork: "",
    duration: 0,
    access: "playable",
  };
}
export async function searchTracks(q, signal) {
  const result = await api(
    `/search?q=${encodeURIComponent(q)}&type=tracks&limit=40`,
    { signal },
  );
  return collectionItems(result).map(soundcloudTrack);
}
