import { canonicalHistoryTrack } from "./history.js";

const MAX_PLAYLISTS = 50;
const MAX_PLAYLIST_TRACKS = 500;
const MAX_QUEUED_PLAYLIST_MUTATIONS = 100;
const playlistIDPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

const generatedId = () => crypto.randomUUID();
const text = (value) => String(value ?? "").trim();

export function canonicalPlaylistID(value) {
  const id = text(value).toLowerCase();
  return playlistIDPattern.test(id) ? id : "";
}

// A Mixora playlist is a user-owned ordered set. The track converter is shared
// with history so queued playlist writes use the same provider-neutral refs as
// the server's compact TrackSnapshot.
export function canonicalPlaylist(value) {
  if (!value || typeof value !== "object") return null;
  const id = canonicalPlaylistID(value.id);
  const name = text(value.name);
  const description = text(value.description);
  if (!id || !name || name.length > 120 || description.length > 2000) return null;

  const tracks = [];
  const seen = new Set();
  for (const raw of Array.isArray(value.tracks) ? value.tracks : []) {
    const track = canonicalHistoryTrack(raw);
    if (!track) return null;
    const key = `${track.source}:${track.id}`;
    if (seen.has(key)) return null;
    seen.add(key);
    tracks.push(track);
    if (tracks.length > MAX_PLAYLIST_TRACKS) return null;
  }
  return {
    id,
    name,
    ...(description ? { description } : {}),
    tracks,
    pinned: value.pinned === true,
    liked: value.liked === true,
  };
}

export function playlistState(payload) {
  const values = Array.isArray(payload)
    ? payload
    : Array.isArray(payload?.playlists)
      ? payload.playlists
      : [];
  const result = [];
  const seen = new Set();
  for (const value of values) {
    const playlist = canonicalPlaylist(value);
    if (!playlist || seen.has(playlist.id)) continue;
    seen.add(playlist.id);
    result.push(playlist);
    if (result.length === MAX_PLAYLISTS) break;
  }
  return result;
}

export function createPlaylistMutation(playlist, options = {}) {
  const normalized = canonicalPlaylist(playlist);
  const id = options.id || generatedId;
  const idempotencyKey = text(typeof id === "function" ? id() : id);
  if (!normalized || !idempotencyKey) return null;
  return {
    type: "replace",
    idempotency_key: idempotencyKey,
    playlist: normalized,
  };
}

export function createPlaylistDeleteMutation(playlistID, options = {}) {
  const id = canonicalPlaylistID(playlistID);
  const generator = options.id || generatedId;
  const idempotencyKey = text(
    typeof generator === "function" ? generator() : generator,
  );
  if (!id || !idempotencyKey) return null;
  return {
    type: "delete",
    idempotency_key: idempotencyKey,
    playlist_id: id,
  };
}

export function normalizePlaylistMutation(value) {
  const type = text(value?.type).toLowerCase();
  const idempotencyKey = text(value?.idempotency_key);
  if (!idempotencyKey) return null;
  if (type === "replace") {
    const playlist = canonicalPlaylist(value.playlist);
    return playlist
      ? { type, idempotency_key: idempotencyKey, playlist }
      : null;
  }
  if (type === "delete") {
    const playlistID = canonicalPlaylistID(value.playlist_id);
    return playlistID
      ? { type, idempotency_key: idempotencyKey, playlist_id: playlistID }
      : null;
  }
  return null;
}

export const playlistQueueKey = (userId) =>
  `mixora-ui:playlist-queue:${String(userId || "guest")}`;
export const playlistBackfillMarkerKey = (userId) =>
  `mixora-ui:playlist-backfill-v1:${String(userId || "guest")}`;

const mutationTarget = (mutation) =>
  mutation.type === "replace" ? mutation.playlist.id : mutation.playlist_id;

const normalizedQueue = (queue) => {
  const result = [];
  for (const raw of Array.isArray(queue) ? queue : []) {
    const mutation = normalizePlaylistMutation(raw);
    if (!mutation) continue;
    const target = mutationTarget(mutation);
    const index = result.findIndex((item) => mutationTarget(item) === target);
    if (index >= 0) result.splice(index, 1);
    result.push(mutation);
  }
  return result.slice(-MAX_QUEUED_PLAYLIST_MUTATIONS);
};

// Keep only the newest full desired state for an individual playlist. This
// coalesces a burst of drag/drop edits while preserving mutations for other
// playlists in their original order.
export function enqueuePlaylistMutation(queue, value) {
  const mutation = normalizePlaylistMutation(value);
  const current = normalizedQueue(queue);
  if (!mutation) return current;
  const target = mutationTarget(mutation);
  return [...current.filter((item) => mutationTarget(item) !== target), mutation].slice(
    -MAX_QUEUED_PLAYLIST_MUTATIONS,
  );
}

export function playlistBatch(queue, limit = 1) {
  const max = Number.isFinite(limit) ? Math.max(0, Math.floor(limit)) : 1;
  return max ? normalizedQueue(queue).slice(0, max) : [];
}

export function acknowledgePlaylistMutations(queue, delivered) {
  const deliveredKeys = new Set(
    (Array.isArray(delivered) ? delivered : [])
      .map((value) =>
        typeof value === "string" ? text(value) : text(value?.idempotency_key),
      )
      .filter(Boolean),
  );
  return normalizedQueue(queue).filter(
    (mutation) => !deliveredKeys.has(mutation.idempotency_key),
  );
}

export function playlistRequest(value) {
  const mutation = normalizePlaylistMutation(value);
  if (!mutation) return null;
  if (mutation.type === "delete") {
    return {
      method: "DELETE",
      path: `/me/playlists/${encodeURIComponent(mutation.playlist_id)}`,
      body: { idempotency_key: mutation.idempotency_key },
    };
  }
  const { playlist } = mutation;
  return {
    method: "PUT",
    path: `/me/playlists/${encodeURIComponent(playlist.id)}`,
    body: {
      idempotency_key: mutation.idempotency_key,
      name: playlist.name,
      description: playlist.description || "",
      tracks: playlist.tracks,
      pinned: playlist.pinned,
      liked: playlist.liked,
    },
  };
}

function withDerivedPins(library, playlists) {
  const ownIDs = new Set(playlists.map((playlist) => playlist.id));
  const oldPins = Array.isArray(library?.pins) ? library.pins : [];
  const foreignPins = oldPins.filter((id) => !ownIDs.has(id));
  return {
    ...library,
    playlists,
    pins: [
      ...playlists.filter((playlist) => playlist.pinned).map((playlist) => playlist.id),
      ...foreignPins,
    ],
  };
}

// The durable account list is authoritative after a successful GET. Queued
// local mutations overlay it so the user never sees a just-created playlist
// disappear while offline.
export function mergePlaylists(
  library,
  remote,
  pending = [],
  { authoritative = true } = {},
) {
  const base = authoritative
    ? playlistState(remote)
    : playlistState(library?.playlists);
  const result = [...base];
  for (const mutation of normalizedQueue(pending)) {
    const id = mutationTarget(mutation);
    const index = result.findIndex((playlist) => playlist.id === id);
    if (mutation.type === "delete") {
      if (index >= 0) result.splice(index, 1);
      continue;
    }
    if (index >= 0) result[index] = mutation.playlist;
    else result.unshift(mutation.playlist);
  }
  return withDerivedPins(library, result.slice(0, MAX_PLAYLISTS));
}

// The browser used to store locally-created playlists inside the broad library
// snapshot. Migrate those only when the account has no normalized playlists;
// a non-empty server list, including a migrated list with different UUIDs, is
// authoritative and must not be duplicated.
export function legacyPlaylistBackfill(library, options = {}) {
  const pins = new Set(Array.isArray(library?.pins) ? library.pins : []);
  const create = options.create || createPlaylistMutation;
  const result = [];
  for (const raw of Array.isArray(library?.playlists) ? library.playlists : []) {
    const mutation = create({ ...raw, pinned: raw?.pinned === true || pins.has(raw?.id) });
    if (mutation) result.push(mutation);
    if (result.length === MAX_PLAYLISTS) break;
  }
  return result;
}
