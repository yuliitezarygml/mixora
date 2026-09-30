const eventID = () => crypto.randomUUID();
const timestamp = () => new Date().toISOString();

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
    result_count: Array.isArray(tracks) ? tracks.length : 0,
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
