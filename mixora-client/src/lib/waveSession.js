import { trackKey } from "./library.js";
import { normalizeWave } from "./wave.js";

const MAX_TRACK_SESSIONS = 100;
const MAX_CONTEXT_TEXT = 500;
const MAX_MODEL_VERSION = 120;
const MAX_ROUND = 100_000;
const sessionIDPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const controlCharacters = /[\u0000-\u001F\u007F]/g;

const text = (value, limit) =>
  String(value ?? "")
    .replace(controlCharacters, "")
    .trim()
    .slice(0, limit);

const boundedText = (value, limit) => {
  const raw = String(value ?? "")
    .replace(controlCharacters, "")
    .trim();
  return raw.length <= limit ? raw : "";
};

const validSessionID = (value) => {
  const sessionID = boundedText(value, 36).toLowerCase();
  return sessionIDPattern.test(sessionID) ? sessionID : "";
};

const validTrackKey = (value) => {
  const key = boundedText(value, 300);
  const separator = key.indexOf(":");
  return separator > 0 && separator < key.length - 1 ? key : "";
};

const trackKeyOf = (track) => {
  if (
    !track ||
    typeof track !== "object" ||
    !text(track.source, 40) ||
    !text(track.id, 240)
  ) {
    return "";
  }
  return validTrackKey(trackKey(track));
};

const validRound = (value) => {
  const round = Number(value);
  return Number.isSafeInteger(round) && round >= 0 && round <= MAX_ROUND
    ? round
    : 0;
};

export const waveSessionStorageKey = (userID) =>
  `mixora-ui:wave-session-v1:${String(userID || "guest")}`;

export function normalizeWaveContext(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const result = {};
  for (const field of ["artist", "genre", "title"]) {
    const current = text(value[field], MAX_CONTEXT_TEXT);
    if (current) result[field] = current;
  }
  return result;
}

const sessionEntries = (value) => {
  const entries = value instanceof Map ? [...value.entries()] : value;
  const result = new Map();
  const boundedEntries = Array.isArray(entries)
    ? entries.slice(-MAX_TRACK_SESSIONS * 2)
    : [];
  for (const entry of boundedEntries) {
    const [rawKey, rawSessionID] = Array.isArray(entry) ? entry : [];
    const key = validTrackKey(rawKey);
    const sessionID = validSessionID(rawSessionID);
    if (!key || !sessionID) continue;
    // A later batch is the authoritative owner if a recommender happens to
    // return an already seen track again.
    result.delete(key);
    result.set(key, sessionID);
  }
  while (result.size > MAX_TRACK_SESSIONS) {
    result.delete(result.keys().next().value);
  }
  return result;
};

// updateWaveTrackSessions keeps the persisted feedback ownership small and
// detached from provider grammar. When a local fallback has no server session,
// its tracks deliberately lose a potentially stale old session association.
export function updateWaveTrackSessions(current, tracks, sessionID) {
  const result = sessionEntries(current);
  const normalizedSessionID = validSessionID(sessionID);
  for (const track of Array.isArray(tracks) ? tracks : []) {
    const key = trackKeyOf(track);
    if (!key) continue;
    result.delete(key);
    if (normalizedSessionID) result.set(key, normalizedSessionID);
  }
  while (result.size > MAX_TRACK_SESSIONS) {
    result.delete(result.keys().next().value);
  }
  return result;
}

// The snapshot contains no cookie, playback URL, or full track metadata. The
// durable server still validates every feedback event against its own
// recommendation_impressions rows.
export function createWaveSessionSnapshot({
  modelVersion,
  preferences,
  context,
  round,
  trackSessions,
} = {}) {
  return {
    version: 1,
    model_version:
      text(modelVersion || "rules-v0", MAX_MODEL_VERSION) || "rules-v0",
    preferences: normalizeWave(preferences),
    context: normalizeWaveContext(context),
    round: validRound(round),
    track_sessions: [...sessionEntries(trackSessions).entries()],
  };
}

// Restore only mappings for tracks that were restored into the player queue.
// A stale local snapshot therefore cannot attach a Wave session to an unrelated
// track selected later in another browser/device.
export function restoreWaveSession(value, queue) {
  if (
    !value ||
    typeof value !== "object" ||
    value.version !== 1 ||
    !Array.isArray(value.track_sessions)
  ) {
    return null;
  }
  const queueKeys = new Set(
    (Array.isArray(queue) ? queue : []).map(trackKeyOf).filter(Boolean),
  );
  if (!queueKeys.size) return null;
  const sessions = new Map(
    [...sessionEntries(value.track_sessions)].filter(([key]) =>
      queueKeys.has(key),
    ),
  );
  return {
    modelVersion: text(value.model_version, MAX_MODEL_VERSION) || "rules-v0",
    preferences: normalizeWave(value.preferences),
    context: normalizeWaveContext(value.context),
    round: validRound(value.round),
    trackSessions: sessions,
  };
}
