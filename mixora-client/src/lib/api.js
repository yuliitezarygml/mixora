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

const displayName = (value) =>
  typeof value === "string" ? value.trim().normalize("NFC") : "";
export function soundcloudTrack(t) {
  return {
    id: String(t.urn || t.id),
    source: "soundcloud",
    title: displayName(t.title) || "Без названия",
    artist:
      displayName(t.metadata_artist) ||
      displayName(t.publisher_metadata?.artist) ||
      displayName(t.user?.username) ||
      "Исполнитель",
    artistId: String(t.user?.urn || t.user?.id || ""),
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
    id: String(data.urn || data.id),
    name: displayName(data.username),
    artwork: data.avatar_url || "",
    description: data.description || "",
    permalink: data.permalink_url || "",
    followers: data.followers_count || 0,
  };
}
export function soundcloudPlaylist(data) {
  return {
    id: String(data.urn || data.id),
    name: displayName(data.title),
    artwork: data.artwork_url || data.tracks?.[0]?.artwork_url || "",
    description: data.description || "",
    artist: displayName(data.user?.username),
    artistId: String(data.user?.urn || data.user?.id || ""),
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
  const url = value?.stream_url || value?.url || "";
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

export async function getTrackPlayback(id, signal) {
  const data = await api(
    `/tracks/${encodeURIComponent(soundcloudResourceId(id))}/stream`,
    { signal },
  );
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
