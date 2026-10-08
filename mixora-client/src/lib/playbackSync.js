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
        // Preserve the stable provider page, never an expiring audio URL.
        // YouTube, VK and Bandcamp playback re-resolve this after reload or
        // desktop sync.
        permalink: track.permalink,
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

// A delayed provider resolver can outlive the browser's transient user
// gesture. In that case the stream is already loaded and only play() must be
// retried from the next click. Re-resolve only when no source exists or the
// media element itself rejected that source.
export function shouldResolvePlayback({ hasSource, mediaError }) {
  return !hasSource || Boolean(mediaError);
}

export function playbackSocketURL(locationLike) {
  const source = locationLike || globalThis.location;
  const protocol = source?.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${source?.host || ""}/api/v1/playback/ws`;
}

export function decodePlaybackState(payload) {
  let message = payload;
  if (typeof payload === "string") {
    try {
      message = JSON.parse(payload);
    } catch {
      return null;
    }
  }
  if (
    !message ||
    typeof message !== "object" ||
    message.type !== "state" ||
    !message.track ||
    typeof message.track !== "object" ||
    message.track.id === undefined ||
    message.track.id === null ||
    String(message.track.id).trim() === ""
  ) {
    return null;
  }
  return message;
}

export function shouldApplyRemotePosition(
  currentPosition,
  remotePosition,
  threshold = 2,
) {
  return (
    Number.isFinite(currentPosition) &&
    Number.isFinite(remotePosition) &&
    remotePosition >= 0 &&
    Math.abs(currentPosition - remotePosition) > threshold
  );
}

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
