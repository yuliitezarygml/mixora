export class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
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
    throw new ApiError(
      messages[response.status] || data.error || "Не удалось выполнить запрос.",
      response.status,
    );
  }
  return data;
}
export const post = (path, body) =>
  api(path, {
    method: "POST",
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
export const put = (path, body) =>
  api(path, { method: "PUT", body: JSON.stringify(body) });
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
export const collectionItems = (data) =>
  Array.isArray(data) ? data : data?.collection || [];
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
    `/providers/soundcloud/catalog/${kind}?q=${encodeURIComponent(q)}&limit=40`,
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
export const catalogResource = (kind, id, section = "", signal) =>
  api(
    `/providers/soundcloud/catalog/${kind}/${encodeURIComponent(id)}${section ? `/${section}` : ""}`,
    { signal },
  );
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
    `/providers/soundcloud/tracks?q=${encodeURIComponent(q)}&limit=40`,
    { signal },
  );
  return (result.collection || result || []).map(soundcloudTrack);
}
