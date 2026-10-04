const MAX_QUEUE_TRACKS = 30;

const slim = (track) =>
  track
    ? {
        id: track.id,
        source: track.source,
        title: track.title,
        artist: track.artist,
        artistId: track.artistId,
        artwork: track.artwork,
        duration: track.duration,
        access: track.access,
        explicit: !!track.explicit,
      }
    : null;

const sameTrack = (first, second) =>
  first &&
  second &&
  first.source === second.source &&
  String(first.id) === String(second.id);

// Keep the current track inside every persisted/transmitted queue window. A
// long Wave queue used to be sliced from zero, which could restore a current
// item that was no longer in its own list after track 30.
const queueWindow = (current, queue, preferredIndex) => {
  const tracks = Array.isArray(queue) ? queue : [];
  const fromIndex = Number.isSafeInteger(preferredIndex) ? preferredIndex : -1;
  const currentIndex = sameTrack(tracks[fromIndex], current)
    ? fromIndex
    : tracks.findIndex((track) => sameTrack(track, current));
  const start = Math.max(
    0,
    Math.min(
      currentIndex >= 0 ? currentIndex : 0,
      tracks.length - MAX_QUEUE_TRACKS,
    ),
  );
  const result = tracks.slice(start, start + MAX_QUEUE_TRACKS);
  if (current && !result.some((track) => sameTrack(track, current))) {
    result.unshift(current);
    return result.slice(0, MAX_QUEUE_TRACKS);
  }
  return result;
};

const roundedPosition = (position) =>
  Number.isFinite(position) ? Math.round(position * 10) / 10 : 0;

export function playbackSnapshot(current, playing, position, queue) {
  return {
    type: "state",
    playing: !!playing,
    position: roundedPosition(position),
    track: slim(current),
    queue: queueWindow(current, queue).map(slim).filter(Boolean),
  };
}

export function playerStorageSnapshot(current, index, position, queue) {
  const window = queueWindow(current, queue, index);
  const currentIndex = window.findIndex((track) => sameTrack(track, current));
  return {
    track: slim(current),
    queue: window.map(slim).filter(Boolean),
    index: Math.max(0, currentIndex),
    position: roundedPosition(position),
  };
}
