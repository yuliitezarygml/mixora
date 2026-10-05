// Track identity is used while reconciling optimistic browser state. Return an
// empty key for a malformed/legacy UI entry rather than throwing while a user
// clicks a valid nearby result (search history also contains query-only rows).
export const trackKey = (track) => {
  if (!track || typeof track !== "object") return "";
  const source = String(track.source ?? "")
    .trim()
    .toLowerCase();
  const id = String(track.id ?? "").trim();
  return source && id ? `${source}:${id}` : "";
};

// Collections contain more than tracks. Keep their identity provider-aware as
// well: Spotify, SoundCloud and an imported source can legitimately expose
// the same opaque ID. `fallbackSource` preserves browser data saved before
// providers were added (those entries were SoundCloud-only).
export function entityKey(entity, fallbackSource = "soundcloud") {
  if (!entity || typeof entity !== "object") return "";
  const source = String(entity.source || fallbackSource)
    .trim()
    .toLowerCase();
  const id = String(entity.id ?? "").trim();
  return source && id ? `${source}:${id}` : "";
}

// A track detail route can outlive the transient search catalog. Rebuild its
// compact render/playback model from durable user-owned snapshots too, while
// retaining a strict provider-aware identity check so same-shaped IDs from two
// catalogs never bleed into each other.
export function findKnownTrack(catalog, library, source, id) {
  const key = entityKey({ source, id });
  if (!key) return null;
  const state = library && typeof library === "object" ? library : {};
  const candidates = [
    ...(Array.isArray(catalog) ? catalog : []),
    ...(Array.isArray(state.likes) ? state.likes : []),
    ...(Array.isArray(state.dislikes) ? state.dislikes : []),
    ...(Array.isArray(state.history) ? state.history : []),
    ...(Array.isArray(state.playlists)
      ? state.playlists.flatMap((playlist) => playlist?.tracks || [])
      : []),
    ...(Array.isArray(state.listens)
      ? state.listens.map((listen) => listen?.track || listen)
      : []),
    ...(Array.isArray(state.searches)
      ? state.searches.map((entry) => entry?.track).filter(Boolean)
      : []),
  ];
  return candidates.find((track) => entityKey(track) === key) || null;
}

export function duration(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return "0:00";
  return `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, "0")}`;
}
export function uniqueTracks(tracks) {
  return [...new Map(tracks.map((t) => [trackKey(t), t])).values()];
}
export function shuffleTracks(tracks, currentIndex, random = Math.random) {
  const remaining = tracks.filter((_, i) => i !== currentIndex);
  for (let i = remaining.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [remaining[i], remaining[j]] = [remaining[j], remaining[i]];
  }
  return tracks[currentIndex]
    ? [tracks[currentIndex], ...remaining]
    : remaining;
}
export function readStorage(key, fallback) {
  try {
    const value = JSON.parse(localStorage.getItem(key));
    return value === null ? fallback : value;
  } catch {
    return fallback;
  }
}
const libraryLimits = {
  likes: 400,
  dislikes: 400,
  history: 100,
  playlists: 50,
  artists: 200,
  searches: 40,
  savedPlaylists: 100,
  albums: 100,
  episodes: 50,
  listens: 200,
  pins: 40,
};
export function libraryPayload(library) {
  return Object.fromEntries(
    Object.entries(libraryLimits).map(([key, limit]) => {
      const items = Array.isArray(library?.[key])
        ? library[key].slice(0, limit)
        : [];
      return [
        key,
        key === "playlists"
          ? items.map((playlist) => ({
              ...playlist,
              tracks: (playlist.tracks || []).slice(0, 200),
            }))
          : items,
      ];
    }),
  );
}
export function libraryCount(library) {
  return Object.keys(libraryLimits)
    .filter((key) => key !== "listens" && key !== "pins")
    .reduce((sum, key) => sum + (library?.[key]?.length || 0), 0);
}
export function saveStorage(key, value) {
  try {
    localStorage.setItem(key, JSON.stringify(value));
    return true;
  } catch {
    /* Storage may be disabled or full; playback remains available. */
    return false;
  }
}
