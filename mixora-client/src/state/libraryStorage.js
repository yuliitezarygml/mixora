import { useRef, useState } from "react";
import { put } from "../lib/api.js";
import {
  entityKey,
  libraryPayload,
  readStorage,
  saveStorage,
} from "../lib/library.js";
import {
  trackPreferenceBackfillKey,
  trackPreferenceKey,
  trackPreferenceQueueKey,
  trackPreferenceState,
} from "../lib/trackPreferences.js";

export const emptyLibrary = () => ({
  likes: [],
  dislikes: [],
  history: [],
  playlists: [],
  artists: [],
  searches: [],
  savedPlaylists: [],
  albums: [],
  episodes: [],
  listens: [],
  pins: null,
});

const mixoraPlaylistKey = (playlist) => entityKey(playlist, "mixora");

export const storedPinKey = (value, library) => {
  if (value && typeof value === "object") return entityKey(value);
  const raw = String(value ?? "").trim();
  if (!raw) return "";
  // Current pin values are provider-aware. Older browser snapshots carried
  // bare IDs, so recover their intended source from the stored collection.
  if (raw.includes(":")) return raw;
  const own = (library?.playlists || []).find((item) => item.id === raw);
  if (own) return mixoraPlaylistKey(own);
  const saved = (library?.savedPlaylists || []).find((item) => item.id === raw);
  return saved ? entityKey(saved) : `mixora:${raw}`;
};

export function withPins(library) {
  const values = Array.isArray(library.pins)
    ? library.pins
    : (library.playlists || []).map(mixoraPlaylistKey);
  return {
    ...library,
    pins: [
      ...new Set(values.map((value) => storedPinKey(value, library))),
    ].filter(Boolean),
  };
}

export const loadLibrary = (key) =>
  withPins({ ...emptyLibrary(), ...readStorage(key, {}) });

// Preferences, listening history and account playlists have their own
// normalized APIs. A delayed transition-library write must never put stale
// copies of those states back into the server snapshot.
export const withoutDedicatedStateSnapshot = (library) => ({
  ...library,
  likes: [],
  dislikes: [],
  history: [],
  playlists: [],
});

export const transitionSafeLibraryPayload = (library) =>
  libraryPayload(withoutDedicatedStateSnapshot(library));

export const storedList = (key) => {
  const value = readStorage(key, []);
  return Array.isArray(value) ? value : [];
};

export const pendingTrackPreferencesFor = (userId, remote = []) => {
  const remoteKeys = new Set(
    trackPreferenceState(remote).map((value) =>
      trackPreferenceKey(value.track),
    ),
  );
  const queue = storedList(trackPreferenceQueueKey(userId));
  const backfill = storedList(trackPreferenceBackfillKey(userId)).filter(
    (mutation) => !remoteKeys.has(trackPreferenceKey(mutation?.track)),
  );
  return { queue, backfill };
};

// Keeps library persistence separate from the application orchestration. The
// public surface deliberately mirrors the old AppContext state so consumers
// remain unchanged while state domains move into dedicated hooks incrementally.
export function useLibraryStorage(user, userRef) {
  const scope = user?.id || "guest";
  const storageKey = `mixora-ui:library:${scope}`;
  const [stored, setStored] = useState(() => ({
    key: storageKey,
    value: loadLibrary(storageKey),
  }));
  const library =
    stored.key === storageKey ? stored.value : loadLibrary(storageKey);
  const saveTimer = useRef(0);

  const updateLibrary = (fn, { sync = true } = {}) => {
    setStored((previous) => {
      const base =
        previous.key === storageKey ? previous.value : loadLibrary(storageKey);
      const next = fn(base);
      saveStorage(storageKey, next);
      if (userRef.current && sync) {
        clearTimeout(saveTimer.current);
        saveTimer.current = setTimeout(() => {
          saveTimer.current = 0;
          // Likes/dislikes now have a dedicated desired-state API. Never let a
          // delayed full-library snapshot roll that state back on another device.
          put("/library", transitionSafeLibraryPayload(next)).catch(() => {});
        }, 600);
      }
      return { key: storageKey, value: next };
    });
  };

  return { library, saveTimer, setStored, storageKey, updateLibrary };
}
