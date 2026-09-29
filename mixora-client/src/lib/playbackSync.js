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

export function playbackSnapshot(current, playing, position, queue) {
  return {
    type: "state",
    playing: !!playing,
    position: Number.isFinite(position) ? Math.round(position * 10) / 10 : 0,
    track: slim(current),
    queue: (queue || []).slice(0, 30).map(slim).filter(Boolean),
  };
}
