export const trackKey = (t) => `${t.source}:${t.id}`;
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
  } catch {
    /* Storage may be disabled or full; playback remains available. */
  }
}
