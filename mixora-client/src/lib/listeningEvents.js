const eventID = () => crypto.randomUUID();
const timestamp = () => new Date().toISOString();
const meaningfulSeekSeconds = 5;

const milliseconds = (value) =>
  Number.isFinite(value) && value >= 0 ? Math.round(value * 1000) : undefined;

const eventTrackId = (track) => {
  const value = String(track?.id ?? "").trim();
  if ((track?.source || "music").toLowerCase() !== "soundcloud") return value;
  return value.match(/(?:^|:|\/)(\d+)$/)?.[1] || value;
};

export function trackListeningEvent(type, track, extra = {}, options = {}) {
  const trackId = eventTrackId(track);
  if (!type || !trackId) return null;
  return {
    idempotency_key: (options.id || eventID)(),
    type,
    track_source: track.source || "music",
    track_id: trackId,
    occurred_at: (options.now || timestamp)(),
    ...extra,
  };
}

// Slider input can emit many tiny moves. Keep the durable event log useful by
// recording only deliberate jumps while preserving the before/after position.
export function seekListeningEvent(
  track,
  fromSeconds,
  toSeconds,
  durationSeconds,
  options = {},
) {
  if (
    !Number.isFinite(fromSeconds) ||
    !Number.isFinite(toSeconds) ||
    Math.abs(toSeconds - fromSeconds) < meaningfulSeekSeconds
  ) {
    return null;
  }
  return trackListeningEvent(
    "seek",
    track,
    {
      position_ms: milliseconds(toSeconds),
      duration_ms: milliseconds(durationSeconds),
      context: { from_position_ms: milliseconds(fromSeconds) },
    },
    options,
  );
}

export function searchListeningEvents(query, tracks = [], options = {}) {
  const value = String(query || "").trim();
  if (!value) return [];
  const id = options.id || eventID;
  const now = options.now || timestamp;
  const occurredAt = now();
  const visible = (Array.isArray(tracks) ? tracks : [])
    .filter((track) => track?.id)
    .slice(0, 10);
  const context = {
    query: value,
    result_count:
      options.resultCount ?? (Array.isArray(tracks) ? tracks.length : 0),
    visible_count: visible.length,
    kind: options.kind || "tracks",
    source: options.source || "music",
  };
  return [
    {
      idempotency_key: id(),
      type: "search",
      track_source: "",
      track_id: "",
      occurred_at: occurredAt,
      context,
    },
    ...visible.map((track, rank) => ({
      idempotency_key: id(),
      type: "impression",
      track_source: track.source || "music",
      track_id: eventTrackId(track),
      occurred_at: occurredAt,
      context: { ...context, rank: rank + 1, surface: "search" },
    })),
  ];
}
