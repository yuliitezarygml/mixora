import { useRef } from "react";
import { api, put } from "../lib/api.js";
import { readStorage, saveStorage } from "../lib/library.js";
import { loadLibrary, pendingTrackPreferencesFor } from "./libraryStorage.js";
import {
  acknowledgeTrackPreferences,
  legacyTrackPreferenceBackfill,
  preferenceBatch,
  refillTrackPreferenceQueue,
  replaceTrackPreferenceState,
  trackPreferenceBackfillKey,
  trackPreferenceBackfillMarkerKey,
  trackPreferenceKey,
  trackPreferenceQueueKey,
  trackPreferenceRequest,
  trackPreferenceState,
  mergeTrackPreferences,
} from "../lib/trackPreferences.js";

// Owns the server-authoritative desired-state sync for likes/dislikes. The
// AppProvider keeps user lifecycle orchestration, while this hook guarantees
// account fences, legacy migration and offline retry behaviour stay together.
export function useTrackPreferenceSync(userRef, setStored) {
  const preferenceFlushRef = useRef(false);
  const preferenceLoadRef = useRef("");
  const preferenceStateRef = useRef({
    userId: "",
    loaded: false,
    settled: false,
    values: [],
  });

  const applyTrackPreferenceState = (
    userId,
    remote,
    { authoritative = true } = {},
  ) => {
    if (userRef.current?.id !== userId) return;
    const key = `mixora-ui:library:${userId}`;
    const remoteState = trackPreferenceState(remote);
    const remoteKeys = new Set(
      remoteState.map((value) => trackPreferenceKey(value.track)),
    );
    const { queue, backfill } = pendingTrackPreferencesFor(userId, remoteState);
    if (remoteKeys.size) {
      saveStorage(trackPreferenceBackfillKey(userId), backfill);
    }
    setStored((previous) => {
      const base = previous.key === key ? previous.value : loadLibrary(key);
      const next = mergeTrackPreferences(
        base,
        remoteState,
        [...queue, ...backfill],
        {
          authoritative,
        },
      );
      saveStorage(key, next);
      return { key, value: next };
    });
  };

  const refillPendingTrackPreferences = (userId) => {
    const queueKey = trackPreferenceQueueKey(userId);
    const backfillKey = trackPreferenceBackfillKey(userId);
    const next = refillTrackPreferenceQueue(
      readStorage(queueKey, []),
      readStorage(backfillKey, []),
    );
    saveStorage(queueKey, next.queue);
    saveStorage(backfillKey, next.backfill);
    return preferenceBatch(next.queue, 1);
  };

  const enqueueLegacyTrackPreferences = (userId, remote) => {
    // A current state (including neutral) is authoritative. Only an empty
    // server list is eligible for the one-time browser-local migration.
    if (
      trackPreferenceState(remote).length ||
      readStorage(trackPreferenceBackfillMarkerKey(userId), false) === true
    ) {
      return false;
    }
    const queueKey = trackPreferenceQueueKey(userId);
    const backfillKey = trackPreferenceBackfillKey(userId);
    const queued = readStorage(queueKey, []);
    const storedBackfill = readStorage(backfillKey, []);
    const candidates =
      Array.isArray(storedBackfill) && storedBackfill.length
        ? storedBackfill
        : legacyTrackPreferenceBackfill(
            loadLibrary(`mixora-ui:library:${userId}`),
          );
    if (!candidates.length) return false;

    const next = refillTrackPreferenceQueue(queued, candidates);
    const candidateKeys = new Set(
      candidates
        .map((mutation) => mutation?.track)
        .map((track) => `${track?.source || ""}:${track?.id || ""}`)
        .filter((key) => key !== ":"),
    );
    const persistedKeys = new Set(
      [...next.queue, ...next.backfill]
        .map((mutation) => mutation?.track)
        .map((track) => `${track?.source || ""}:${track?.id || ""}`)
        .filter((key) => key !== ":"),
    );
    const queuedKeys = new Set(
      preferenceBatch(queued).map(
        (mutation) => `${mutation.track.source}:${mutation.track.id}`,
      ),
    );
    const covered = [...candidateKeys].every(
      (key) => persistedKeys.has(key) || queuedKeys.has(key),
    );
    if (!covered) return false;

    const queueSaved = saveStorage(queueKey, next.queue);
    const backfillSaved = saveStorage(backfillKey, next.backfill);
    if (!queueSaved || !backfillSaved) return false;
    // Write this only after every eligible legacy item has a durable queued
    // representation (or was superseded by an already queued newer choice).
    return saveStorage(trackPreferenceBackfillMarkerKey(userId), true);
  };

  const flushTrackPreferences = async (userId = userRef.current?.id) => {
    if (
      !userId ||
      userRef.current?.id !== userId ||
      preferenceFlushRef.current ||
      navigator.onLine === false
    ) {
      return;
    }
    const key = trackPreferenceQueueKey(userId);
    const [mutation] = refillPendingTrackPreferences(userId);
    if (!mutation) return;
    preferenceFlushRef.current = true;
    let delivered = false;
    try {
      const result = await put(
        "/me/track-preferences",
        trackPreferenceRequest(mutation),
      );
      const remaining = acknowledgeTrackPreferences(readStorage(key, []), [
        mutation,
      ]);
      saveStorage(key, remaining);
      const [saved] = trackPreferenceState([result]);
      if (saved) {
        const state = preferenceStateRef.current;
        if (state.userId === userId && state.loaded) {
          preferenceStateRef.current = {
            ...state,
            values: replaceTrackPreferenceState(state.values, saved),
          };
        }
        applyTrackPreferenceState(userId, [saved], { authoritative: false });
      }
      delivered = true;
    } catch {
      // Keep the latest desired state and its idempotency key for reconnect.
    } finally {
      preferenceFlushRef.current = false;
      const activeUserId = userRef.current?.id;
      if (activeUserId && activeUserId !== userId) {
        queueMicrotask(() => flushTrackPreferences(activeUserId));
      } else if (delivered && refillPendingTrackPreferences(userId).length) {
        queueMicrotask(() => flushTrackPreferences(userId));
      }
    }
  };

  const loadTrackPreferences = async (userId = userRef.current?.id) => {
    if (
      !userId ||
      userRef.current?.id !== userId ||
      preferenceLoadRef.current === userId
    ) {
      return false;
    }
    preferenceLoadRef.current = userId;
    try {
      const response = await api("/me/track-preferences");
      if (userRef.current?.id !== userId) return false;
      const values = trackPreferenceState(response);
      enqueueLegacyTrackPreferences(userId, values);
      preferenceStateRef.current = {
        userId,
        loaded: true,
        settled: true,
        values,
      };
      applyTrackPreferenceState(userId, values);
      void flushTrackPreferences(userId);
      return true;
    } catch {
      // Keep browser-local state usable when the desired-state endpoint is
      // unavailable. Its mutation queue is retained for the next reconnect.
      if (userRef.current?.id !== userId) return false;
      preferenceStateRef.current = {
        userId,
        loaded: false,
        settled: true,
        values: [],
      };
      void flushTrackPreferences(userId);
      return false;
    } finally {
      if (preferenceLoadRef.current === userId) {
        preferenceLoadRef.current = "";
      }
    }
  };

  return {
    applyTrackPreferenceState,
    flushTrackPreferences,
    loadTrackPreferences,
    preferenceStateRef,
  };
}
