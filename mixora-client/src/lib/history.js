import { soundcloudResourceId } from "./api.js";

const MAX_QUEUED_HISTORY_RECORDS = 500;
const MAX_HISTORY_ENTRIES = 100;

const generatedId = () => crypto.randomUUID();
const timestamp = () => new Date().toISOString();

const text = (value) => String(value ?? "").trim();

const isoTimestamp = (value) => {
  const raw = typeof value === "string" ? value.trim() : "";
  if (!raw) return "";
  const parsed = Date.parse(raw);
  return Number.isFinite(parsed) ? new Date(parsed).toISOString() : "";
};

// History uses the same compact, provider-neutral track snapshot as the
// server. Keeping this conversion at the queue boundary means an offline
// retry sends the exact same record that was shown optimistically to the user.
export function canonicalHistoryTrack(track) {
  if (!track || typeof track !== "object") return null;
  const source = text(track.source).toLowerCase();
  const rawId = text(track.id);
  const title = text(track.title);
  const artist = text(track.artist);
  if (!source || !rawId || !title || !artist) return null;

  let id = rawId;
  let artistId = text(track.artistId);
  if (source === "soundcloud") {
    try {
      id = soundcloudResourceId(rawId);
      artistId = artistId ? soundcloudResourceId(artistId) : "";
    } catch {
      return null;
    }
  }

  const duration = Number(track.duration);
  return {
    id,
    source,
    title,
    artist,
    ...(artistId ? { artistId } : {}),
    ...(text(track.artwork) ? { artwork: text(track.artwork) } : {}),
    ...(Number.isFinite(duration) && duration >= 0 ? { duration } : {}),
    ...(track.explicit === true ? { explicit: true } : {}),
    ...(text(track.access) ? { access: text(track.access) } : {}),
    ...(text(track.permalink) ? { permalink: text(track.permalink) } : {}),
  };
}

export function historyTrackKey(track) {
  const canonical = canonicalHistoryTrack(track);
  return canonical ? `${canonical.source}:${canonical.id}` : "";
}

export function createHistoryRecord(track, options = {}) {
  const canonicalTrack = canonicalHistoryTrack(track);
  const id = options.id || generatedId;
  const idempotencyKey = text(typeof id === "function" ? id() : id);
  const now = options.now || timestamp;
  const occurredAt = isoTimestamp(
    typeof now === "function" ? now() : now,
  );
  if (!canonicalTrack || !idempotencyKey || !occurredAt) return null;
  return {
    idempotency_key: idempotencyKey,
    track: canonicalTrack,
    occurred_at: occurredAt,
  };
}

export function normalizeHistoryRecord(record) {
  const track = canonicalHistoryTrack(record?.track);
  const idempotencyKey = text(record?.idempotency_key);
  const occurredAt = isoTimestamp(record?.occurred_at);
  if (!track || !idempotencyKey || !occurredAt) return null;
  return {
    idempotency_key: idempotencyKey,
    track,
    occurred_at: occurredAt,
  };
}

// Keep queue-only fields out of the request shape so future local bookkeeping
// cannot accidentally become part of the idempotency fingerprint on the API.
export function historyRequest(record) {
  const normalized = normalizeHistoryRecord(record);
  if (!normalized) return null;
  return {
    idempotency_key: normalized.idempotency_key,
    track: normalized.track,
    occurred_at: normalized.occurred_at,
  };
}

export const historyQueueKey = (userId) =>
  `mixora-ui:history-queue:${String(userId || "guest")}`;

const queueLimit = (limit) => {
  if (!Number.isFinite(limit)) return MAX_QUEUED_HISTORY_RECORDS;
  return Math.max(0, Math.floor(limit));
};

const normalizedQueue = (queue) =>
  (Array.isArray(queue) ? queue : [])
    .map(normalizeHistoryRecord)
    .filter(Boolean)
    .reduce(
      (items, record) => [
        ...items.filter(
          (item) => item.idempotency_key !== record.idempotency_key,
        ),
        record,
      ],
      [],
    );

// Each meaningful listen is intentionally retained, even when it is the same
// track: the server uses it to advance `play_count`. Only a retry with the
// same idempotency key replaces its own queued record.
export function enqueueHistoryRecord(
  queue,
  record,
  limit = MAX_QUEUED_HISTORY_RECORDS,
) {
  const normalized = normalizedQueue(queue);
  const next = normalizeHistoryRecord(record);
  const updated = next
    ? [
        ...normalized.filter(
          (item) => item.idempotency_key !== next.idempotency_key,
        ),
        next,
      ]
    : normalized;
  const cap = queueLimit(limit);
  return cap ? updated.slice(-cap) : [];
}

export function historyBatch(queue, limit = 1) {
  const cap = queueLimit(limit);
  return cap ? normalizedQueue(queue).slice(0, cap) : [];
}

export function acknowledgeHistoryRecords(queue, delivered) {
  const ids = new Set(
    (Array.isArray(delivered) ? delivered : [])
      .map((item) =>
        typeof item === "string"
          ? text(item)
          : text(item?.idempotency_key),
      )
      .filter(Boolean),
  );
  return normalizedQueue(queue)
    .filter((record) => !ids.has(record.idempotency_key))
    .slice(-MAX_QUEUED_HISTORY_RECORDS);
}

export function normalizeHistoryEntry(value) {
  const track = canonicalHistoryTrack(value?.track);
  const firstListenedAt = isoTimestamp(value?.first_listened_at);
  const lastListenedAt = isoTimestamp(value?.last_listened_at);
  const playCount = Number(value?.play_count);
  if (
    !track ||
    !firstListenedAt ||
    !lastListenedAt ||
    !Number.isSafeInteger(playCount) ||
    playCount < 1
  ) {
    return null;
  }
  return {
    track,
    first_listened_at: firstListenedAt,
    last_listened_at: lastListenedAt,
    play_count: playCount,
  };
}

export function historyEntries(payload) {
  const entries = Array.isArray(payload)
    ? payload
    : Array.isArray(payload?.history)
      ? payload.history
      : [];
  return entries.map(normalizeHistoryEntry).filter(Boolean);
}

const entryTime = (entry) => Date.parse(entry.last_listened_at) || 0;

const sortHistoryEntries = (entries) =>
  entries.sort((first, second) => {
    const difference = entryTime(second) - entryTime(first);
    if (difference) return difference;
    return historyTrackKey(first.track).localeCompare(historyTrackKey(second.track));
  });

// Combine full server entries without inflating their play counters. This is
// used when a PUT finishes while a GET is in flight: the newer aggregate wins.
export function mergeHistoryEntries(...groups) {
  const entries = new Map();
  for (const group of groups) {
    for (const entry of historyEntries(group)) {
      const key = historyTrackKey(entry.track);
      const current = entries.get(key);
      if (!current) {
        entries.set(key, entry);
        continue;
      }
      const incomingIsNewer = entryTime(entry) >= entryTime(current);
      entries.set(key, {
        track: incomingIsNewer ? entry.track : current.track,
        first_listened_at:
          entry.first_listened_at < current.first_listened_at
            ? entry.first_listened_at
            : current.first_listened_at,
        last_listened_at:
          entryTime(entry) >= entryTime(current)
            ? entry.last_listened_at
            : current.last_listened_at,
        play_count: Math.max(entry.play_count, current.play_count),
      });
    }
  }
  return sortHistoryEntries([...entries.values()]).slice(0, MAX_HISTORY_ENTRIES);
}

const entryFromRecord = (record) => {
  const normalized = normalizeHistoryRecord(record);
  if (!normalized) return null;
  return {
    track: normalized.track,
    first_listened_at: normalized.occurred_at,
    last_listened_at: normalized.occurred_at,
    play_count: 1,
  };
};

const mergeQueuedRecords = (entries, pending) => {
  const result = new Map(
    entries.map((entry) => [historyTrackKey(entry.track), entry]),
  );
  for (const rawRecord of normalizedQueue(pending)) {
    const record = entryFromRecord(rawRecord);
    if (!record) continue;
    const key = historyTrackKey(record.track);
    const current = result.get(key);
    if (!current) {
      result.set(key, record);
      continue;
    }
    const recordIsNewer = entryTime(record) >= entryTime(current);
    result.set(key, {
      track: recordIsNewer ? record.track : current.track,
      first_listened_at:
        record.first_listened_at < current.first_listened_at
          ? record.first_listened_at
          : current.first_listened_at,
      last_listened_at: recordIsNewer
        ? record.last_listened_at
        : current.last_listened_at,
      play_count: current.play_count + 1,
    });
  }
  return sortHistoryEntries([...result.values()]).slice(0, MAX_HISTORY_ENTRIES);
};

// The GET endpoint is authoritative for durable history. Pending local PUTs
// are overlaid afterwards, so an offline listen never flashes out of the UI.
export function mergeHistory(
  library,
  remote,
  pending = [],
  { authoritative = true } = {},
) {
  const remoteEntries = mergeHistoryEntries(remote);
  const entries = mergeQueuedRecords(remoteEntries, pending);
  const currentHistory = Array.isArray(library?.history) ? library.history : [];
  const fallbackEntries = currentHistory
    .map((track) => {
      const canonical = canonicalHistoryTrack(track);
      if (!canonical) return null;
      return {
        track: canonical,
        first_listened_at: "2000-01-01T00:00:00.000Z",
        last_listened_at: "2000-01-01T00:00:00.000Z",
        play_count: 1,
      };
    })
    .filter(Boolean);
  const visible = authoritative
    ? entries
    : mergeHistoryEntries(entries, fallbackEntries);
  return {
    ...library,
    history: visible.map((entry) => entry.track),
  };
}
