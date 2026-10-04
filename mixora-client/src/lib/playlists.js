import { canonicalHistoryTrack } from "./history.js";
import { entityKey } from "./library.js";

const MAX_PLAYLISTS = 50;
const MAX_PLAYLIST_TRACKS = 500;
const MAX_QUEUED_PLAYLIST_MUTATIONS = 100;
const MAX_LEGACY_PLAYLIST_ID = 120;
const playlistIDPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

const generatedId = () => crypto.randomUUID();
const text = (value) => String(value ?? "").trim();

const validRevision = (value) => {
  const revision = Number(value);
  return Number.isSafeInteger(revision) && revision >= 0 ? revision : null;
};

const legacyID = (value) => {
  const id = text(value);
  return id && id.length <= MAX_LEGACY_PLAYLIST_ID ? id : "";
};

const legacyPlaylistID = (value, index) => {
  const id = legacyID(value?.id);
  return id ? id : `legacy-snapshot-${index + 1}`;
};

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
  const importID = legacyID(value.legacy_id);
  const revision = Object.hasOwn(value, "revision")
    ? validRevision(value.revision)
    : 0;
  if (
    !id ||
    !name ||
    name.length > 120 ||
    description.length > 2000 ||
    revision === null
  )
    return null;

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
    ...(importID ? { legacy_id: importID } : {}),
    name,
    ...(description ? { description } : {}),
    tracks,
    pinned: value.pinned === true,
    liked: value.liked === true,
    revision,
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
  const importID = legacyID(options.legacyID);
  if (!normalized || !idempotencyKey || (importID && normalized.revision !== 0))
    return null;
  return {
    type: "replace",
    idempotency_key: idempotencyKey,
    playlist: normalized,
    ...(importID ? { legacy_id: importID } : {}),
  };
}

export function createPlaylistDeleteMutation(playlist, options = {}) {
  const source = playlist && typeof playlist === "object" ? playlist : null;
  const id = canonicalPlaylistID(source?.id ?? playlist);
  const expectedRevision = source
    ? validRevision(source.revision ?? 0)
    : validRevision(options.expectedRevision ?? 0);
  const generator = options.id || generatedId;
  const idempotencyKey = text(
    typeof generator === "function" ? generator() : generator,
  );
  if (!id || !idempotencyKey || expectedRevision === null) return null;
  return {
    type: "delete",
    idempotency_key: idempotencyKey,
    playlist_id: id,
    expected_revision: expectedRevision,
  };
}

export function normalizePlaylistMutation(value) {
  const type = text(value?.type).toLowerCase();
  const idempotencyKey = text(value?.idempotency_key);
  if (!idempotencyKey) return null;
  if (type === "replace") {
    const playlist = canonicalPlaylist(value.playlist);
    const importID = legacyID(value.legacy_id);
    if (importID && playlist?.revision !== 0) return null;
    return playlist
      ? {
          type,
          idempotency_key: idempotencyKey,
          playlist,
          ...(importID ? { legacy_id: importID } : {}),
        }
      : null;
  }
  if (type === "delete") {
    const playlistID = canonicalPlaylistID(value.playlist_id);
    const expectedRevision = Object.hasOwn(value ?? {}, "expected_revision")
      ? validRevision(value.expected_revision)
      : 0;
    return playlistID && expectedRevision !== null
      ? {
          type,
          idempotency_key: idempotencyKey,
          playlist_id: playlistID,
          expected_revision: expectedRevision,
        }
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
  return [
    ...current.filter((item) => mutationTarget(item) !== target),
    mutation,
  ].slice(-MAX_QUEUED_PLAYLIST_MUTATIONS);
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

export function discardPlaylistMutations(queue, playlistID) {
  const target = canonicalPlaylistID(playlistID);
  if (!target) return normalizedQueue(queue);
  return normalizedQueue(queue).filter(
    (mutation) => mutationTarget(mutation) !== target,
  );
}

// A mutation created while the preceding write was in flight still describes
// the user's latest intent. After that write succeeds, move it to the new
// revision so the same device does not conflict with its own serialized edit.
export function rebasePlaylistMutations(queue, playlistID, revision) {
  const target = canonicalPlaylistID(playlistID);
  const nextRevision = validRevision(revision);
  if (!target || nextRevision === null) return normalizedQueue(queue);
  return normalizedQueue(queue).map((mutation) => {
    if (mutationTarget(mutation) !== target) return mutation;
    if (mutation.type === "replace") {
      return {
        ...mutation,
        playlist: { ...mutation.playlist, revision: nextRevision },
      };
    }
    return { ...mutation, expected_revision: nextRevision };
  });
}

export function playlistRequest(value) {
  const mutation = normalizePlaylistMutation(value);
  if (!mutation) return null;
  if (mutation.type === "delete") {
    return {
      method: "DELETE",
      path: `/me/playlists/${encodeURIComponent(mutation.playlist_id)}`,
      body: {
        idempotency_key: mutation.idempotency_key,
        expected_revision: mutation.expected_revision,
      },
    };
  }
  const { playlist } = mutation;
  return {
    method: "PUT",
    path: `/me/playlists/${encodeURIComponent(playlist.id)}`,
    body: {
      idempotency_key: mutation.idempotency_key,
      expected_revision: playlist.revision,
      name: playlist.name,
      description: playlist.description || "",
      tracks: playlist.tracks,
      pinned: playlist.pinned,
      liked: playlist.liked,
      ...(mutation.legacy_id ? { legacy_id: mutation.legacy_id } : {}),
    },
  };
}

function withDerivedPins(library, playlists) {
  const ownKeys = new Set(
    playlists.map((playlist) => entityKey(playlist, "mixora")),
  );
  const oldPins = Array.isArray(library?.pins) ? library.pins : [];
  const foreignPins = oldPins
    .map((value) => {
      const raw = String(value ?? "").trim();
      if (!raw) return "";
      if (raw.includes(":")) return raw;
      const saved = (library?.savedPlaylists || []).find(
        (playlist) => playlist.id === raw,
      );
      return saved ? entityKey(saved) : `mixora:${raw}`;
    })
    .filter((key) => key && !ownKeys.has(key));
  return {
    ...library,
    playlists,
    pins: [...new Set([
      ...playlists
        .filter((playlist) => playlist.pinned)
        .map((playlist) => entityKey(playlist, "mixora")),
      ...foreignPins,
    ])],
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
// snapshot. Migrate each still-unrepresented item instead of treating a
// non-empty server list as a reason to discard the whole local collection. A
// read-only legacy_id returned by the server identifies entries that migration
// 008 already imported with a different normalized UUID.
export function legacyPlaylistBackfill(library, options = {}) {
  const pins = new Set(
    (Array.isArray(library?.pins) ? library.pins : []).map((value) => {
      const raw = String(value ?? "").trim();
      return raw.includes(":") ? raw : `mixora:${raw}`;
    }),
  );
  const create = options.create || createPlaylistMutation;
  const generateID = options.generateID || generatedId;
  const remote = playlistState(options.remote);
  const remoteIDs = new Set(remote.map((playlist) => playlist.id));
  const remoteLegacyIDs = new Set(
    remote.map((playlist) => text(playlist.legacy_id)).filter(Boolean),
  );
  const result = [];
  const values = Array.isArray(library?.playlists) ? library.playlists : [];
  for (const [index, raw] of values.entries()) {
    const sourceLegacyID = legacyPlaylistID(raw, index);
    if (remoteLegacyIDs.has(sourceLegacyID)) continue;

    let id = canonicalPlaylistID(raw?.id);
    if (!id) {
      const generated =
        typeof generateID === "function" ? generateID() : generateID;
      id = canonicalPlaylistID(generated);
    }
    if (!id || remoteIDs.has(id)) continue;

    const mutation = create(
      {
        ...raw,
        id,
        revision: 0,
        pinned:
          raw?.pinned === true || pins.has(entityKey(raw, "mixora")),
      },
      { legacyID: sourceLegacyID },
    );
    if (mutation) result.push(mutation);
    if (result.length === MAX_PLAYLISTS) break;
  }
  return result;
}
