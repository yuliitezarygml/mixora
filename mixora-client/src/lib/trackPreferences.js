import { soundcloudResourceId } from "./api.js";

const preferences = new Set(["liked", "disliked", "neutral"]);
const MAX_QUEUED_PREFERENCES = 100;
const MAX_LEGACY_PREFERENCE_BACKFILL = 800;

const generatedId = () => crypto.randomUUID();

export function isTrackPreference(value) {
  return preferences.has(value);
}

export function canonicalPreferenceTrack(track) {
  if (!track || typeof track !== "object") return null;
  const source = String(track.source ?? "")
    .trim()
    .toLowerCase();
  const rawId = String(track.id ?? "").trim();
  const title = String(track.title ?? "").trim();
  const artist = String(track.artist ?? "").trim();
  if (!source || !rawId || !title || !artist) return null;

  let id = rawId;
  if (source === "soundcloud") {
    try {
      id = soundcloudResourceId(rawId);
    } catch {
      return null;
    }
  }
  const optionalText = (value) => String(value ?? "").trim();
  const duration = Number(track.duration);
  return {
    id,
    source,
    title,
    artist,
    ...(optionalText(track.artistId)
      ? { artistId: optionalText(track.artistId) }
      : {}),
    ...(optionalText(track.artwork)
      ? { artwork: optionalText(track.artwork) }
      : {}),
    ...(Number.isFinite(duration) && duration >= 0 ? { duration } : {}),
    ...(track.explicit === true ? { explicit: true } : {}),
    ...(optionalText(track.access)
      ? { access: optionalText(track.access) }
      : {}),
    ...(optionalText(track.permalink)
      ? { permalink: optionalText(track.permalink) }
      : {}),
  };
}

export function createTrackPreferenceMutation(track, preference, options = {}) {
  const canonicalTrack = canonicalPreferenceTrack(track);
  if (!canonicalTrack || !isTrackPreference(preference)) return null;

  const id = options.id || generatedId;
  const idempotencyKey = String(typeof id === "function" ? id() : id).trim();
  if (!idempotencyKey) return null;

  return {
    idempotency_key: idempotencyKey,
    track: canonicalTrack,
    preference,
  };
}

export function normalizeTrackPreferenceMutation(mutation) {
  const track = canonicalPreferenceTrack(mutation?.track);
  const idempotencyKey = String(mutation?.idempotency_key ?? "").trim();
  if (!track || !idempotencyKey || !isTrackPreference(mutation?.preference)) {
    return null;
  }
  return {
    idempotency_key: idempotencyKey,
    track,
    preference: mutation.preference,
    ...(mutation?.migration === true ? { migration: true } : {}),
  };
}

// Keep local migration bookkeeping out of the strict PUT contract.
export function trackPreferenceRequest(mutation) {
  const normalized = normalizeTrackPreferenceMutation(mutation);
  if (!normalized) return null;
  return {
    idempotency_key: normalized.idempotency_key,
    track: normalized.track,
    preference: normalized.preference,
  };
}

export function trackPreferenceKey(track) {
  const canonicalTrack = canonicalPreferenceTrack(track);
  return canonicalTrack ? `${canonicalTrack.source}:${canonicalTrack.id}` : "";
}

export const trackPreferenceQueueKey = (userId) =>
  `mixora-ui:track-preferences:${String(userId || "guest")}`;

// Browser-only libraries predate the server-owned desired-state endpoint. Keep
// their one-time bridge separate from the regular mutation queue so a large
// local collection cannot evict newer clicks made while offline.
export const trackPreferenceBackfillKey = (userId) =>
  `mixora-ui:track-preference-backfill:${String(userId || "guest")}`;

export const trackPreferenceBackfillMarkerKey = (userId) =>
  `mixora-ui:track-preference-backfill-v1:${String(userId || "guest")}`;

export function normalizeTrackPreferenceState(value) {
  const track = canonicalPreferenceTrack(value?.track);
  const preference = value?.preference;
  if (!track || !isTrackPreference(preference)) return null;
  const revision = Number(value?.revision);
  const updatedAt =
    typeof value?.updated_at === "string" ? value.updated_at : "";
  return {
    track,
    preference,
    ...(Number.isSafeInteger(revision) && revision > 0 ? { revision } : {}),
    ...(updatedAt ? { updated_at: updatedAt } : {}),
  };
}

// The endpoint returns every current state, including neutral records. Accept a
// bare array too so an older client transport envelope cannot reintroduce the
// legacy library snapshot as the preference source of truth.
export function trackPreferenceState(payload) {
  const values = Array.isArray(payload)
    ? payload
    : Array.isArray(payload?.preferences)
      ? payload.preferences
      : [];
  return values.map(normalizeTrackPreferenceState).filter(Boolean);
}

export function replaceTrackPreferenceState(state, value) {
  const entry = normalizeTrackPreferenceState(value);
  if (!entry) return trackPreferenceState(state);
  const key = trackPreferenceKey(entry.track);
  return [
    entry,
    ...trackPreferenceState(state).filter(
      (current) => trackPreferenceKey(current.track) !== key,
    ),
  ];
}

export function applyTrackPreference(library, value) {
  const entry = normalizeTrackPreferenceState(value);
  if (!entry) return library;
  const key = trackPreferenceKey(entry.track);
  const withoutTrack = (tracks) =>
    (Array.isArray(tracks) ? tracks : []).filter(
      (track) => trackPreferenceKey(track) !== key,
    );
  const likes = withoutTrack(library?.likes);
  const dislikes = withoutTrack(library?.dislikes);
  if (entry.preference === "liked") {
    return { ...library, likes: [entry.track, ...likes], dislikes };
  }
  if (entry.preference === "disliked") {
    return { ...library, likes, dislikes: [entry.track, ...dislikes] };
  }
  return { ...library, likes, dislikes };
}

// A loaded server state replaces only the legacy likes/dislikes snapshot. Any
// unsent local mutation is applied afterwards, so reconnecting cannot roll an
// optimistic click back while its idempotent PUT is still queued.
export function mergeTrackPreferences(
  library,
  remote,
  pending = [],
  { authoritative = true } = {},
) {
  let merged = authoritative
    ? { ...library, likes: [], dislikes: [] }
    : {
        ...library,
        likes: Array.isArray(library?.likes) ? library.likes : [],
        dislikes: Array.isArray(library?.dislikes) ? library.dislikes : [],
      };
  const current = Array.isArray(remote) ? remote : trackPreferenceState(remote);
  // List endpoint is newest first; replay oldest-to-newest to preserve that
  // ordering in the user-visible collections.
  for (const value of current.slice().reverse()) {
    merged = applyTrackPreference(merged, value);
  }
  for (const mutation of normalizedQueue(pending)) {
    merged = applyTrackPreference(merged, mutation);
  }
  return merged;
}

const queueLimit = (limit) => {
  if (!Number.isFinite(limit)) return MAX_QUEUED_PREFERENCES;
  return Math.max(0, Math.floor(limit));
};

const keepLatestPreference = (queue, mutation) => {
  const key = trackPreferenceKey(mutation.track);
  return [
    ...queue.filter(
      (item) =>
        item.idempotency_key !== mutation.idempotency_key &&
        trackPreferenceKey(item.track) !== key,
    ),
    mutation,
  ];
};

const normalizedQueue = (queue) =>
  (Array.isArray(queue) ? queue : [])
    .map(normalizeTrackPreferenceMutation)
    .filter(Boolean)
    .reduce(keepLatestPreference, []);

// Convert the old browser snapshot into current-state writes. We send each
// collection oldest-to-newest so that a later GET preserves its familiar order.
// A legacy duplicate in both lists ends as `disliked`, matching the previous
// UI behavior where a dislike always removed a like.
export function legacyTrackPreferenceBackfill(library, options = {}) {
  let result = [];
  const append = (track, preference) => {
    const mutation = createTrackPreferenceMutation(track, preference, options);
    if (mutation) {
      result = keepLatestPreference(result, { ...mutation, migration: true });
    }
  };
  const tracks = (field) =>
    (Array.isArray(library?.[field]) ? library[field] : [])
      .slice(0, MAX_LEGACY_PREFERENCE_BACKFILL)
      .reverse();

  for (const track of tracks("likes")) append(track, "liked");
  for (const track of tracks("dislikes")) append(track, "disliked");
  return result.slice(-MAX_LEGACY_PREFERENCE_BACKFILL);
}

// Moves as much of a persisted migration bridge into the delivery queue as can
// fit, without evicting a newer normal mutation. A normal mutation for the
// same track wins and consumes the legacy bridge entry.
export function refillTrackPreferenceQueue(
  queue,
  backfill,
  limit = MAX_QUEUED_PREFERENCES,
) {
  const cap = queueLimit(limit);
  const next = cap ? normalizedQueue(queue).slice(-cap) : [];
  const queuedKeys = new Set(
    next.map((item) => trackPreferenceKey(item.track)),
  );
  const remaining = [];

  for (const mutation of normalizedQueue(backfill)) {
    const key = trackPreferenceKey(mutation.track);
    if (queuedKeys.has(key)) continue;
    if (next.length < cap) {
      next.push(mutation);
      queuedKeys.add(key);
    } else {
      remaining.push(mutation);
    }
  }
  return { queue: next, backfill: remaining };
}

export function removeTrackPreferenceMutation(queue, track) {
  const key = trackPreferenceKey(track);
  if (!key) return normalizedQueue(queue);
  return normalizedQueue(queue).filter(
    (mutation) => trackPreferenceKey(mutation.track) !== key,
  );
}

// If an optimistic user click fills an already-full delivery queue, preserve
// only displaced migration records. Regular offline choices retain the normal
// bounded-queue behavior, while the one-time bridge cannot silently lose an
// older local like or dislike.
export function preserveDisplacedTrackPreferenceMigrations(
  backfill,
  previousQueue,
  nextQueue,
  replacedTrack,
) {
  const replacementKey = trackPreferenceKey(replacedTrack);
  const deliveredOrQueued = new Set(
    normalizedQueue(nextQueue).map((mutation) => mutation.idempotency_key),
  );
  const displaced = normalizedQueue(previousQueue).filter(
    (mutation) =>
      mutation.migration === true &&
      !deliveredOrQueued.has(mutation.idempotency_key) &&
      trackPreferenceKey(mutation.track) !== replacementKey,
  );
  return normalizedQueue([
    ...displaced,
    ...removeTrackPreferenceMutation(backfill, replacedTrack),
  ]).slice(-MAX_LEGACY_PREFERENCE_BACKFILL);
}

// Entries are chronological: replacing a preference moves its latest desired
// state to the tail, so delivery remains predictable after reconnecting.
export function enqueueTrackPreference(
  queue,
  mutation,
  limit = MAX_QUEUED_PREFERENCES,
) {
  const normalized = normalizedQueue(queue);
  const next = normalizeTrackPreferenceMutation(mutation);
  const updated = next ? keepLatestPreference(normalized, next) : normalized;
  const cap = queueLimit(limit);
  return cap ? updated.slice(-cap) : [];
}

export function preferenceBatch(queue, limit = MAX_QUEUED_PREFERENCES) {
  const cap = queueLimit(limit);
  return cap ? normalizedQueue(queue).slice(0, cap) : [];
}

export function acknowledgeTrackPreferences(queue, delivered) {
  const deliveredIds = new Set(
    (Array.isArray(delivered) ? delivered : [])
      .map((item) =>
        typeof item === "string"
          ? item.trim()
          : String(item?.idempotency_key ?? "").trim(),
      )
      .filter(Boolean),
  );
  return normalizedQueue(queue)
    .filter((item) => !deliveredIds.has(item.idempotency_key))
    .slice(-MAX_QUEUED_PREFERENCES);
}
