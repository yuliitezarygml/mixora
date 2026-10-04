import { useEffect, useRef, useState } from "react";
import { AppContext as Context } from "./context.js";
import { api, getTrackPlayback, post, put } from "../lib/api.js";
import {
  readStorage,
  saveStorage,
  entityKey,
  trackKey,
  uniqueTracks,
  shuffleTracks,
  libraryPayload,
  libraryCount,
} from "../lib/library.js";
import { dropAccountToken, rememberAccount } from "../lib/accounts.js";
import {
  playbackSnapshot,
  playerStorageSnapshot,
} from "../lib/playbackSync.js";
import {
  acknowledgeEvents,
  enqueueEvent,
  eventBatch,
  eventQueueKey,
} from "../lib/eventQueue.js";
import {
  acknowledgeHistoryRecords,
  bindHistoryGeneration,
  createHistoryRecord,
  enqueueHistoryRecord,
  historyBatch,
  historyEntries,
  historyQueueKey,
  historyRequest,
  historySnapshot,
  mergeHistory,
  mergeHistoryEntries,
  normalizeHistoryEntry,
} from "../lib/history.js";
import {
  acknowledgePlaylistMutations,
  createPlaylistDeleteMutation,
  createPlaylistMutation,
  discardPlaylistMutations,
  enqueuePlaylistMutation,
  legacyPlaylistBackfill,
  mergePlaylists,
  playlistBackfillMarkerKey,
  playlistBatch,
  playlistQueueKey,
  playlistRequest,
  playlistState,
  rebasePlaylistMutations,
} from "../lib/playlists.js";
import { buildWave, defaultWave } from "../lib/wave.js";
import { waveFeedbackPath, withWaveSession } from "../lib/waveFeedback.js";
import { buildWaveRequest, decodeWaveResponse } from "../lib/waveRequest.js";
import {
  createWaveSessionSnapshot,
  restoreWaveSession,
  updateWaveTrackSessions,
  waveSessionStorageKey,
} from "../lib/waveSession.js";
import {
  acknowledgeTrackPreferences,
  createTrackPreferenceMutation,
  enqueueTrackPreference,
  legacyTrackPreferenceBackfill,
  mergeTrackPreferences,
  preferenceBatch,
  preserveDisplacedTrackPreferenceMigrations,
  refillTrackPreferenceQueue,
  replaceTrackPreferenceState,
  trackPreferenceBackfillKey,
  trackPreferenceBackfillMarkerKey,
  trackPreferenceKey,
  trackPreferenceQueueKey,
  trackPreferenceRequest,
  trackPreferenceState,
} from "../lib/trackPreferences.js";
import {
  seekListeningEvent,
  searchListeningEvents,
  trackListeningEvent,
} from "../lib/listeningEvents.js";
import initialCatalog from "../data/catalog.json";
function applyQuality(hls, quality) {
  if (!hls) return;
  const levels = hls.levels || [];
  if (!levels.length || quality === "optimal") {
    hls.currentLevel = -1;
    return;
  }
  hls.currentLevel = quality === "high" ? levels.length - 1 : 0;
}
const emptyLibrary = () => ({
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
const storedPinKey = (value, library) => {
  if (value && typeof value === "object") return entityKey(value);
  const raw = String(value ?? "").trim();
  if (!raw) return "";
  // Current pin values are provider-aware. Older browser snapshots carried
  // bare IDs, so recover their intended source from the stored collection.
  if (raw.includes(":")) return raw;
  const own = (library?.playlists || []).find((item) => item.id === raw);
  if (own) return mixoraPlaylistKey(own);
  const saved = (library?.savedPlaylists || []).find(
    (item) => item.id === raw,
  );
  return saved ? entityKey(saved) : `mixora:${raw}`;
};
function withPins(library) {
  const values = Array.isArray(library.pins)
    ? library.pins
    : (library.playlists || []).map(mixoraPlaylistKey);
  return {
    ...library,
    pins: [...new Set(values.map((value) => storedPinKey(value, library)))].filter(
      Boolean,
    ),
  };
}
const loadLibrary = (key) =>
  withPins({ ...emptyLibrary(), ...readStorage(key, {}) });
// Preferences, listening history and account playlists have their own
// normalized APIs. A delayed transition-library write must never put stale
// copies of those states back into the server snapshot.
const withoutDedicatedStateSnapshot = (library) => ({
  ...library,
  likes: [],
  dislikes: [],
  history: [],
  playlists: [],
});
const transitionSafeLibraryPayload = (library) =>
  libraryPayload(withoutDedicatedStateSnapshot(library));
const storedList = (key) => {
  const value = readStorage(key, []);
  return Array.isArray(value) ? value : [];
};
const pendingTrackPreferencesFor = (userId, remote = []) => {
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
function withStoredPlus(user) {
  if (!user?.id || typeof user.plus === "boolean") return user;
  return {
    ...user,
    plus: readStorage(`mixora-ui:plus:${user.id}`, false) === true,
  };
}
function profileOf(payload) {
  const next = withStoredPlus(payload);
  if (!next) return next;
  const { token, ...profile } = next;
  return profile;
}
function storeAccount(payload, token) {
  const profile = profileOf(payload);
  if (!profile?.id) return profile;
  saveStorage(
    "mixora-ui:accounts",
    rememberAccount(
      readStorage("mixora-ui:accounts", []),
      profile,
      token || payload?.token,
    ),
  );
  return profile;
}
export function AppProvider({ children }) {
  const [user, setUser] = useState(null),
    [sessionReady, setSessionReady] = useState(false),
    [authOpen, setAuthOpen] = useState(false),
    [authIntent, setAuthIntent] = useState("login");
  const [notice, setNotice] = useState(""),
    [panel, setPanel] = useState(null),
    [queue, setQueue] = useState([]),
    [index, setIndex] = useState(-1);
  const [playing, setPlaying] = useState(false),
    [loading, setLoading] = useState(false),
    [position, setPosition] = useState(0),
    [length, setLength] = useState(0),
    [playbackError, setPlaybackError] = useState("");
  const [playbackAttempt, setPlaybackAttempt] = useState(0);
  const [settings, setSettingsState] = useState(() =>
    readStorage("mixora-ui:settings", {
      theme: "dark",
      volume: 0.65,
      explicit: true,
      animation: true,
      autoplay: true,
      playbackRate: 1,
      quality: "optimal",
      equalizer: false,
      wave: defaultWave,
    }),
  );
  const [repeat, setRepeat] = useState("off"),
    [shuffled, setShuffled] = useState(false),
    [catalog, setCatalog] = useState(initialCatalog);
  const [waveActive, setWaveActive] = useState(false),
    [waveBusy, setWaveBusy] = useState(false),
    [waveModelVersion, setWaveModelVersion] = useState("rules-v0"),
    [waveContext, setWaveContext] = useState(null),
    [waveSettingsOpen, setWaveSettingsOpen] = useState(false);
  const waveGeneration = useRef(0),
    waveRound = useRef(0),
    waveFetching = useRef(false),
    waveSession = useRef(""),
    waveTrackSessions = useRef(new Map()),
    heard = useRef(false),
    saveTimer = useRef(0),
    userRef = useRef(null);
  const wavePreferences = { ...defaultWave, ...settings.wave };
  const scope = user?.id || "guest";
  const storageKey = `mixora-ui:library:${scope}`;
  const [stored, setStored] = useState(() => ({
    key: storageKey,
    value: loadLibrary(storageKey),
  }));
  const library =
    stored.key === storageKey ? stored.value : loadLibrary(storageKey);
  userRef.current = user;
  const audioRef = useRef(null),
    hlsRef = useRef(null),
    pending = useRef(null),
    latest = useRef({}),
    resumeRef = useRef(null),
    restoredRef = useRef(""),
    waveRestoredRef = useRef(""),
    eventFlushRef = useRef(false),
    historyFlushRef = useRef(false),
    historyClearingRef = useRef(false),
    historyLoadRef = useRef(""),
    historyStateRef = useRef({
      userId: "",
      loaded: false,
      entries: [],
      generation: 0,
    }),
    playlistFlushRef = useRef(false),
    playlistLoadRef = useRef(""),
    playlistStateRef = useRef({
      userId: "",
      loaded: false,
      values: [],
    }),
    preferenceFlushRef = useRef(false),
    preferenceLoadRef = useRef(""),
    preferenceStateRef = useRef({
      userId: "",
      loaded: false,
      settled: false,
      values: [],
    }),
    eqRef = useRef(null),
    settingsRef = useRef(settings);
  settingsRef.current = settings;
  const current = queue[index] || null;
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
  const pendingHistoryRecords = (userId) => storedList(historyQueueKey(userId));
  const applyHistoryState = (userId, remote, { authoritative = true } = {}) => {
    if (userRef.current?.id !== userId) return;
    const key = `mixora-ui:library:${userId}`;
    const entries = historyEntries(remote);
    setStored((previous) => {
      const base = previous.key === key ? previous.value : loadLibrary(key);
      const next = mergeHistory(base, entries, pendingHistoryRecords(userId), {
        authoritative,
      });
      saveStorage(key, next);
      return { key, value: next };
    });
  };
  const flushHistory = async (userId = userRef.current?.id) => {
    const currentState = historyStateRef.current;
    if (
      !userId ||
      userRef.current?.id !== userId ||
      historyFlushRef.current ||
      historyClearingRef.current ||
      currentState.userId !== userId ||
      !currentState.loaded ||
      navigator.onLine === false
    ) {
      return;
    }
    const key = historyQueueKey(userId);
    const [record] = historyBatch(readStorage(key, []), 1);
    if (!record) return;
    const request = historyRequest(record);
    if (!request) return;

    historyFlushRef.current = true;
    let settled = false;
    try {
      const result = await put("/me/history", request);
      // A clear may have started while this PUT was waiting for the server.
      // Leave the record queued until the clear resolves instead of rendering
      // a just-cleared track back into the account.
      if (historyClearingRef.current) return;
      const entry = normalizeHistoryEntry(result);
      if (!entry) throw Error("История вернула некорректную запись.");

      saveStorage(
        key,
        acknowledgeHistoryRecords(readStorage(key, []), [record]),
      );
      const state = historyStateRef.current;
      if (state.userId === userId) {
        const entries = mergeHistoryEntries(state.entries, [entry]);
        historyStateRef.current = { ...state, entries };
        applyHistoryState(userId, entries, {
          authoritative: state.loaded,
        });
      } else {
        applyHistoryState(userId, [entry], { authoritative: false });
      }
      settled = true;
    } catch (error) {
      if (error?.code === "history_generation_conflict") {
        saveStorage(
          key,
          acknowledgeHistoryRecords(readStorage(key, []), [record]),
        );
        const state = historyStateRef.current;
        if (state.userId === userId) {
          applyHistoryState(userId, state.entries, {
            authoritative: state.loaded,
          });
        }
        settled = true;
        if (!historyClearingRef.current)
          queueMicrotask(() => loadHistory(userId));
        return;
      }
      // Keep the exact idempotency key and occurrence time for the next
      // reconnect. A retry is therefore safe even after a response timeout.
    } finally {
      historyFlushRef.current = false;
      const activeUserId = userRef.current?.id;
      if (activeUserId && activeUserId !== userId) {
        queueMicrotask(() => flushHistory(activeUserId));
      } else if (settled && historyBatch(readStorage(key, []), 1).length) {
        queueMicrotask(() => flushHistory(userId));
      }
    }
  };
  const loadHistory = async (userId = userRef.current?.id) => {
    if (
      !userId ||
      userRef.current?.id !== userId ||
      historyClearingRef.current ||
      historyLoadRef.current === userId
    ) {
      return false;
    }
    historyLoadRef.current = userId;
    try {
      const response = await api("/history?limit=100");
      if (userRef.current?.id !== userId || historyClearingRef.current)
        return false;
      const snapshot = historySnapshot(response);
      const queueKey = historyQueueKey(userId);
      saveStorage(
        queueKey,
        bindHistoryGeneration(readStorage(queueKey, []), snapshot.generation),
      );
      const previous = historyStateRef.current;
      // A PUT can finish while this GET is in flight. Retain the newer PUT
      // aggregate only within the same clear generation. A new generation is
      // an account-wide clear and must remove the old in-memory entries.
      const entries = mergeHistoryEntries(
        snapshot.entries,
        previous.userId === userId &&
          previous.generation === snapshot.generation
          ? previous.entries
          : [],
      );
      historyStateRef.current = {
        userId,
        loaded: true,
        entries,
        generation: snapshot.generation,
      };
      applyHistoryState(userId, entries);
      void flushHistory(userId);
      return true;
    } catch {
      if (userRef.current?.id !== userId) return false;
      const previous = historyStateRef.current;
      historyStateRef.current = {
        userId,
        loaded: false,
        entries: previous.userId === userId ? previous.entries : [],
        generation: previous.userId === userId ? previous.generation : 0,
      };
      // Browser-local history remains usable while the API is unavailable.
      void flushHistory(userId);
      return false;
    } finally {
      if (historyLoadRef.current === userId) historyLoadRef.current = "";
    }
  };
  const clearHistory = async () => {
    const userId = userRef.current?.id;
    if (!userId) {
      setAuthOpen(true);
      return false;
    }
    if (navigator.onLine === false) {
      toast("Подключитесь к сети, чтобы очистить историю аккаунта.");
      return false;
    }
    if (historyClearingRef.current) return false;

    historyClearingRef.current = true;
    try {
      const cleared = await api("/me/history", { method: "DELETE" });
      if (userRef.current?.id !== userId) return false;

      saveStorage(historyQueueKey(userId), []);
      historyStateRef.current = {
        userId,
        loaded: true,
        entries: [],
        generation:
          Number.isSafeInteger(Number(cleared?.generation)) &&
          Number(cleared.generation) >= 0
            ? Number(cleared.generation)
            : historyStateRef.current.generation + 1,
      };
      applyHistoryState(userId, []);
      toast("История прослушивания очищена.");
      return true;
    } catch (error) {
      if (userRef.current?.id === userId)
        toast(error?.message || "Не удалось очистить историю прослушивания.");
      return false;
    } finally {
      historyClearingRef.current = false;
      if (userRef.current?.id === userId)
        queueMicrotask(() => flushHistory(userId));
    }
  };
  const pendingPlaylistMutations = (userId) =>
    storedList(playlistQueueKey(userId));
  const applyPlaylistState = (
    userId,
    remote,
    { authoritative = true } = {},
  ) => {
    if (userRef.current?.id !== userId) return;
    const key = `mixora-ui:library:${userId}`;
    setStored((previous) => {
      const base = previous.key === key ? previous.value : loadLibrary(key);
      const next = mergePlaylists(
        base,
        playlistState(remote),
        pendingPlaylistMutations(userId),
        { authoritative },
      );
      saveStorage(key, next);
      return { key, value: next };
    });
  };
  const enqueueLegacyPlaylists = (userId, remote) => {
    if (readStorage(playlistBackfillMarkerKey(userId), false) === true)
      return false;
    const candidates = legacyPlaylistBackfill(
      loadLibrary(`mixora-ui:library:${userId}`),
      { remote },
    );
    const key = playlistQueueKey(userId);
    let queue = readStorage(key, []);
    for (const mutation of candidates) {
      queue = enqueuePlaylistMutation(queue, mutation);
    }
    if (!saveStorage(key, queue)) return false;
    return saveStorage(playlistBackfillMarkerKey(userId), true);
  };
  const flushPlaylists = async (userId = userRef.current?.id) => {
    if (
      !userId ||
      userRef.current?.id !== userId ||
      playlistFlushRef.current ||
      navigator.onLine === false
    ) {
      return;
    }
    const key = playlistQueueKey(userId);
    const [mutation] = playlistBatch(readStorage(key, []), 1);
    const request = playlistRequest(mutation);
    if (!mutation || !request) return;

    playlistFlushRef.current = true;
    let settled = false;
    try {
      const result =
        request.method === "PUT"
          ? await put(request.path, request.body)
          : await api(request.path, {
              method: request.method,
              body: JSON.stringify(request.body),
            });
      const target =
        mutation.type === "replace"
          ? mutation.playlist.id
          : mutation.playlist_id;
      let remaining = acknowledgePlaylistMutations(readStorage(key, []), [
        mutation,
      ]);
      const state = playlistStateRef.current;
      if (state.userId === userId) {
        let values = state.values;
        if (mutation.type === "replace") {
          const [saved] = playlistState([result]);
          if (saved) {
            remaining = rebasePlaylistMutations(
              remaining,
              target,
              saved.revision,
            );
            values = [
              saved,
              ...values.filter(
                (playlist) =>
                  playlist.id !== saved.id && playlist.id !== target,
              ),
            ];
          }
        } else {
          values = values.filter(
            (playlist) => playlist.id !== mutation.playlist_id,
          );
        }
        saveStorage(key, remaining);
        playlistStateRef.current = { ...state, values };
        applyPlaylistState(userId, values, { authoritative: state.loaded });
      } else {
        saveStorage(key, remaining);
        applyPlaylistState(userId, [], { authoritative: false });
      }
      settled = true;
    } catch (error) {
      const permanent =
        error?.status === 400 ||
        [
          "idempotency_conflict",
          "playlist_limit_reached",
          "playlist_revision_conflict",
        ].includes(error?.code);
      if (!permanent) return;
      const target =
        mutation.type === "replace"
          ? mutation.playlist.id
          : mutation.playlist_id;
      saveStorage(key, discardPlaylistMutations(readStorage(key, []), target));
      const state = playlistStateRef.current;
      if (state.userId === userId) {
        applyPlaylistState(userId, state.values, {
          authoritative: state.loaded,
        });
      }
      if (userRef.current?.id === userId) {
        if (error?.code === "playlist_revision_conflict") {
          toast(
            "Плейлист изменён на другом устройстве. Загрузили актуальную версию.",
          );
          queueMicrotask(() => loadPlaylists(userId));
        } else if (error?.code === "playlist_limit_reached") {
          toast("В аккаунте можно хранить не более 50 плейлистов.");
          queueMicrotask(() => loadPlaylists(userId));
        } else {
          toast(error?.message || "Не удалось сохранить плейлист.");
        }
      }
      settled = true;
    } finally {
      playlistFlushRef.current = false;
      const activeUserId = userRef.current?.id;
      if (activeUserId && activeUserId !== userId) {
        queueMicrotask(() => flushPlaylists(activeUserId));
      } else if (settled && playlistBatch(readStorage(key, []), 1).length) {
        queueMicrotask(() => flushPlaylists(userId));
      }
    }
  };
  const loadPlaylists = async (userId = userRef.current?.id) => {
    if (
      !userId ||
      userRef.current?.id !== userId ||
      playlistLoadRef.current === userId
    ) {
      return false;
    }
    playlistLoadRef.current = userId;
    try {
      const response = await api("/me/playlists");
      if (userRef.current?.id !== userId) return false;
      const values = playlistState(response);
      enqueueLegacyPlaylists(userId, values);
      playlistStateRef.current = { userId, loaded: true, values };
      applyPlaylistState(userId, values);
      void flushPlaylists(userId);
      return true;
    } catch {
      if (userRef.current?.id !== userId) return false;
      playlistStateRef.current = { userId, loaded: false, values: [] };
      // The old local collection stays visible until a successful account GET.
      void flushPlaylists(userId);
      return false;
    } finally {
      if (playlistLoadRef.current === userId) playlistLoadRef.current = "";
    }
  };
  const toast = (message) => setNotice(message);
  const flushEvents = async (userId = userRef.current?.id) => {
    if (!userId || eventFlushRef.current || navigator.onLine === false) return;
    const key = eventQueueKey(userId);
    const batch = eventBatch(readStorage(key, []));
    if (!batch.length) return;
    eventFlushRef.current = true;
    let delivered = false;
    try {
      await post("/events", { events: batch });
      const remaining = acknowledgeEvents(readStorage(key, []), batch);
      saveStorage(key, remaining);
      delivered = true;
    } catch {
      // Events stay queued with their idempotency keys until the next attempt.
    } finally {
      eventFlushRef.current = false;
      if (delivered && eventBatch(readStorage(key, [])).length) {
        queueMicrotask(() => flushEvents(userId));
      }
    }
  };
  const enqueueListeningEvents = (input, sessionId = "") => {
    const userId = userRef.current?.id;
    if (!userId || !input?.length) return;
    const key = eventQueueKey(userId);
    let queued = readStorage(key, []);
    for (const raw of input) {
      const event = sessionId ? withWaveSession(raw, sessionId) : raw;
      queued = enqueueEvent(queued, event);
      const feedbackPath = sessionId
        ? waveFeedbackPath(sessionId, event.type)
        : "";
      if (feedbackPath) void post(feedbackPath, event).catch(() => {});
    }
    saveStorage(key, queued);
    void flushEvents(userId);
  };
  const recordEvent = (type, track, extra = {}) => {
    const event = trackListeningEvent(type, track, extra);
    const sessionId = track
      ? waveTrackSessions.current.get(trackKey(track)) || ""
      : "";
    if (event) enqueueListeningEvents([event], sessionId);
  };
  const recordSearch = (query, tracks, options = {}) =>
    enqueueListeningEvents(searchListeningEvents(query, tracks, options));
  const setSettings = (patch) =>
    setSettingsState((s) => {
      const next = { ...s, ...patch };
      saveStorage("mixora-ui:settings", next);
      return next;
    });
  const remember = (track) => {
    recordEvent("listen_30s", track, {
      position_ms: Math.round((audioRef.current?.currentTime || 0) * 1000),
      duration_ms: Math.round((audioRef.current?.duration || 0) * 1000),
    });
    const userId = userRef.current?.id;
    const historyState = historyStateRef.current;
    const record = createHistoryRecord(track, {
      generation:
        historyState.userId === userId && historyState.loaded
          ? historyState.generation
          : null,
    });
    if (userId && record && !historyClearingRef.current) {
      const key = historyQueueKey(userId);
      const nextQueue = enqueueHistoryRecord(readStorage(key, []), record);
      saveStorage(key, nextQueue);
      // Until the account history has loaded, preserve the visible browser
      // history and overlay the new listen rather than waiting for the API.
      applyHistoryState(userId, [], { authoritative: false });
      void flushHistory(userId);
    }
    updateLibrary((s) => ({
      ...s,
      listens: [{ track, at: Date.now() }, ...s.listens].slice(0, 1000),
    }));
  };
  const play = (track, list = [track], origin = "queue") => {
    if (!track) {
      toast("В подборке пока нет доступных треков.");
      return;
    }
    if (origin !== "wave") {
      waveGeneration.current++;
      waveRound.current = 0;
      waveSession.current = "";
      waveTrackSessions.current.clear();
      waveRestoredRef.current = "";
      const userId = userRef.current?.id;
      if (userId) saveStorage(waveSessionStorageKey(userId), null);
      setWaveModelVersion("rules-v0");
      setWaveActive(false);
      setWaveBusy(false);
    }
    if (!user) {
      pending.current = { track, list };
      setAuthOpen(true);
      return;
    }
    if (track.access === "blocked") {
      toast("Этот трек недоступен для прослушивания.");
      return;
    }
    const allowed = list.filter(
      (t) => t.access !== "blocked" && (settings.explicit || !t.explicit),
    );
    const i = allowed.findIndex((t) => trackKey(t) === trackKey(track));
    if (i < 0) {
      toast("Трек скрыт настройками контента.");
      return;
    }
    setQueue(allowed);
    setIndex(i);
    setPlaybackError("");
    if (current && trackKey(current) === trackKey(track)) {
      if (playbackError || audioRef.current?.error || !audioRef.current?.src) {
        setPlaybackAttempt((attempt) => attempt + 1);
        return;
      }
      audioRef.current
        ?.play()
        .catch(() =>
          setPlaybackError("Нажмите «Воспроизвести», чтобы продолжить."),
        );
    }
  };
  const toggle = () => {
    const a = audioRef.current;
    if (!a || !current) return;
    if (playbackError || a.error || !a.src) {
      setPlaybackAttempt((attempt) => attempt + 1);
      return;
    }
    if (a.paused) {
      a.play().catch(() =>
        setPlaybackError("Не удалось начать воспроизведение."),
      );
    } else a.pause();
  };
  const next = async (auto = false, suppressSkip = false) => {
    if (!queue.length) return;
    if (!auto && !suppressSkip && current) {
      recordEvent("skip", current, {
        position_ms: Math.round((audioRef.current?.currentTime || 0) * 1000),
        duration_ms: Math.round((audioRef.current?.duration || 0) * 1000),
      });
    }
    if (auto && repeat === "one") {
      if (current) {
        recordEvent("repeat", current, {
          position_ms: Math.round((audioRef.current?.currentTime || 0) * 1000),
          duration_ms: Math.round((audioRef.current?.duration || 0) * 1000),
        });
      }
      audioRef.current.currentTime = 0;
      audioRef.current.play().catch(() => {});
      return;
    }
    if (index + 1 < queue.length) setIndex(index + 1);
    else if (waveActive) {
      if (waveFetching.current) return;
      waveFetching.current = true;
      setWaveBusy(true);
      const generation = waveGeneration.current;
      try {
        const result = await loadWave(
          wavePreferences,
          waveContext,
          queue.slice(-12),
        );
        if (generation !== waveGeneration.current) return;
        applyWaveResult(result);
        const tracks = result.tracks;
        if (tracks.length) {
          setQueue((previous) => [...previous, ...tracks]);
          setIndex(queue.length);
        } else {
          setPlaying(false);
          toast(
            "Для этих настроек больше нет треков. Измените настройки волны.",
          );
        }
      } finally {
        waveFetching.current = false;
        if (generation === waveGeneration.current) setWaveBusy(false);
      }
    } else if (repeat === "all") {
      if (queue.length === 1) {
        audioRef.current.currentTime = 0;
        audioRef.current.play().catch(() => {});
      } else setIndex(0);
    } else {
      setPlaying(false);
      audioRef.current?.pause();
    }
  };
  const loadWave = async (preferences, context, exclude = []) => {
    const request = buildWaveRequest({
      library,
      catalog,
      preferences,
      context,
      round: waveRound.current++,
      explicit: settings.explicit,
      exclude,
    });
    const local = () =>
      buildWave(request.seeds, library, preferences, {
        explicit: settings.explicit,
        exclude,
      });
    try {
      const data = await post("/wave", request);
      return {
        ...decodeWaveResponse(data),
        error: null,
      };
    } catch (error) {
      return {
        tracks: local(),
        sessionId: "",
        modelVersion: "rules-v0",
        error,
      };
    }
  };
  const applyWaveResult = (result) => {
    waveSession.current = result.sessionId;
    waveTrackSessions.current = updateWaveTrackSessions(
      waveTrackSessions.current,
      result.tracks,
      result.sessionId,
    );
    setWaveModelVersion(result.modelVersion);
    if (result.tracks.length) {
      setCatalog((previous) => uniqueTracks([...previous, ...result.tracks]));
    }
    if (result.error?.status !== 503 && result.error?.message) {
      toast(result.error.message);
    }
  };
  const startWave = async (context = null, preferences = wavePreferences) => {
    if (!user) {
      pending.current = { wave: { context, preferences } };
      setAuthOpen(true);
      return;
    }
    const generation = ++waveGeneration.current;
    waveSession.current = "";
    waveTrackSessions.current.clear();
    saveStorage(waveSessionStorageKey(user.id), null);
    setWaveModelVersion("rules-v0");
    setWaveActive(false);
    setWaveBusy(true);
    setWaveContext(context);
    setSettings({ wave: preferences });
    try {
      const result = await loadWave(preferences, context);
      if (generation !== waveGeneration.current) return;
      applyWaveResult(result);
      const tracks = result.tracks;
      if (!tracks.length) {
        toast(
          "Для этих настроек нет треков. Попробуйте другой характер или язык.",
        );
        return;
      }
      setWaveActive(true);
      play(tracks[0], tracks, "wave");
    } finally {
      if (generation === waveGeneration.current) setWaveBusy(false);
    }
  };
  const setTrackPreference = (track, preference) => {
    const userId = userRef.current?.id;
    if (!userId) {
      setAuthOpen(true);
      return false;
    }
    const mutation = createTrackPreferenceMutation(track, preference);
    if (!mutation) {
      toast("Не удалось сохранить настройку этого трека.");
      return false;
    }
    const key = trackPreferenceQueueKey(userId);
    const previousQueue = readStorage(key, []);
    const nextQueue = enqueueTrackPreference(previousQueue, mutation);
    saveStorage(key, nextQueue);
    // A fresh user action is newer than any browser-local migration candidate
    // for this track and must not be replayed after the queue drains.
    saveStorage(
      trackPreferenceBackfillKey(userId),
      preserveDisplacedTrackPreferenceMigrations(
        readStorage(trackPreferenceBackfillKey(userId), []),
        previousQueue,
        nextQueue,
        mutation.track,
      ),
    );
    applyTrackPreferenceState(userId, [], { authoritative: false });
    if (
      preferenceStateRef.current.userId === userId &&
      preferenceStateRef.current.settled
    ) {
      void flushTrackPreferences(userId);
    }
    return true;
  };
  const clearTrackPreference = (track) => setTrackPreference(track, "neutral");
  const dislike = (track) => {
    if (!setTrackPreference(track, "disliked")) return;
    if (current && trackKey(current) === trackKey(track)) next(false, true);
    toast("Больше не будем предлагать этот трек в Моей волне");
  };
  const toggleSaved = (field, entity) =>
    updateLibrary((s) => ({
      ...s,
      [field]: s[field].some(
        (item) => entityKey(item) === entityKey(entity),
      )
        ? s[field].filter((item) => entityKey(item) !== entityKey(entity))
        : [entity, ...s[field]],
    }));
  const savePlaylistState = (playlist) => {
    const userId = userRef.current?.id;
    if (!userId) {
      setAuthOpen(true);
      return null;
    }
    const mutation = createPlaylistMutation(playlist);
    if (!mutation) {
      toast("Не удалось сохранить плейлист.");
      return null;
    }
    const key = playlistQueueKey(userId);
    const nextQueue = enqueuePlaylistMutation(readStorage(key, []), mutation);
    if (!saveStorage(key, nextQueue)) {
      toast("Не удалось сохранить изменения плейлиста на этом устройстве.");
      return null;
    }
    applyPlaylistState(userId, [], { authoritative: false });
    void flushPlaylists(userId);
    return mutation.playlist;
  };
  const editPlaylist = (id, patch) => {
    const playlist = library.playlists.find((item) => item.id === id);
    return playlist
      ? savePlaylistState({ ...playlist, ...patch, id: playlist.id })
      : null;
  };
  const removeQueue = (position) => {
    if (position === index) return;
    setQueue((q) => q.filter((_, i) => i !== position));
    if (position < index) setIndex((i) => i - 1);
  };
  const moveQueue = (from, to) => {
    if (from < 0 || to < 0 || from >= queue.length || to >= queue.length)
      return;
    const changed = [...queue];
    const [moved] = changed.splice(from, 1);
    changed.splice(to, 0, moved);
    setQueue(changed);
    setIndex(changed.findIndex((t) => trackKey(t) === trackKey(current)));
  };
  const previous = () => {
    if (audioRef.current?.currentTime > 3) audioRef.current.currentTime = 0;
    else if (index > 0) setIndex(index - 1);
  };
  const seek = (value) => {
    if (audioRef.current && Number.isFinite(value)) {
      const audio = audioRef.current;
      const event = seekListeningEvent(
        current,
        audio.currentTime,
        value,
        audio.duration,
      );
      if (event) {
        const sessionId = current
          ? waveTrackSessions.current.get(trackKey(current)) || ""
          : "";
        enqueueListeningEvents([event], sessionId);
      }
      audio.currentTime = value;
      setPosition(value);
    }
  };
  const toggleLike = (track) => {
    const adding = !library.likes.some(
      (saved) => trackKey(saved) === trackKey(track),
    );
    setTrackPreference(track, adding ? "liked" : "neutral");
  };
  const addQueue = (track) => {
    setQueue((q) => [...q, track]);
    toast("Трек добавлен в очередь");
  };
  const createPlaylist = (name, tracks = []) => {
    if (library.playlists.length >= 50) {
      toast("В аккаунте можно хранить не более 50 плейлистов.");
      return null;
    }
    const p = {
      id: crypto.randomUUID(),
      name: name.trim(),
      tracks,
      pinned: true,
    };
    const saved = savePlaylistState(p);
    if (saved?.tracks.length) {
      for (const track of saved.tracks) {
        recordEvent("add_to_playlist", track, {
          context: { playlist_id: saved.id },
        });
      }
      toast("Плейлист создан, трек добавлен");
    }
    return saved;
  };
  const addToPlaylist = (id, track) => {
    const playlist = library.playlists.find((item) => item.id === id);
    if (!playlist) return false;
    if (playlist.tracks.some((item) => trackKey(item) === trackKey(track))) {
      toast("Этот трек уже есть в плейлисте");
      return false;
    }
    if (!editPlaylist(id, { tracks: [...playlist.tracks, track] }))
      return false;
    recordEvent("add_to_playlist", track, { context: { playlist_id: id } });
    toast("Трек добавлен в плейлист");
    return true;
  };
  const removeFromPlaylist = (id, track) => {
    const playlist = library.playlists.find((item) => item.id === id);
    if (!playlist) return false;
    return Boolean(
      editPlaylist(id, {
        tracks: playlist.tracks.filter(
          (item) => trackKey(item) !== trackKey(track),
        ),
      }),
    );
  };
  const deletePlaylist = (id) => {
    const userId = userRef.current?.id;
    if (!userId) {
      setAuthOpen(true);
      return false;
    }
    const playlist = library.playlists.find((item) => item.id === id);
    const mutation = createPlaylistDeleteMutation(playlist || id);
    if (!mutation) {
      toast("Не удалось удалить плейлист.");
      return false;
    }
    const key = playlistQueueKey(userId);
    if (
      !saveStorage(key, enqueuePlaylistMutation(readStorage(key, []), mutation))
    ) {
      toast("Не удалось сохранить удаление плейлиста на этом устройстве.");
      return false;
    }
    applyPlaylistState(userId, [], { authoritative: false });
    void flushPlaylists(userId);
    return true;
  };
  const togglePin = (value) => {
    const id = typeof value === "object" ? value?.id : value;
    const playlist =
      typeof value === "object" && value?.source
        ? undefined
        : library.playlists.find((item) => item.id === id);
    if (playlist) return editPlaylist(id, { pinned: !playlist.pinned });
    const saved =
      typeof value === "object"
        ? value
        : library.savedPlaylists.find((item) => item.id === id);
    const key = entityKey(saved);
    if (!key) return null;
    return updateLibrary((s) => {
      const pins = s.pins || [];
      return {
        ...s,
        pins: pins.includes(key)
          ? pins.filter((pin) => pin !== key)
          : [key, ...pins],
      };
    });
  };
  const likeOwnPlaylist = (id) => {
    const playlist = library.playlists.find((item) => item.id === id);
    return playlist ? editPlaylist(id, { liked: !playlist.liked }) : null;
  };
  const login = async (email, password) => {
    const payload = await post("/auth/login", { email, password });
    setUser(storeAccount(payload, payload.token));
    setAuthOpen(false);
  };
  const resume = async (token) => {
    try {
      const payload = await post("/auth/resume", { token });
      setUser(storeAccount(payload, payload.token || token));
      setAuthOpen(false);
    } catch (error) {
      if (error.status === 401) {
        const list = readStorage("mixora-ui:accounts", []);
        const owner = list.find((item) => item?.token === token);
        if (owner?.id) {
          saveStorage("mixora-ui:accounts", dropAccountToken(list, owner.id));
        }
      }
      throw error;
    }
  };
  const setPlus = async (plus) => {
    if (!user) {
      setAuthOpen(true);
      return;
    }
    saveStorage(`mixora-ui:plus:${user.id}`, plus);
    const optimistic = { ...user, plus };
    setUser(storeAccount(optimistic));
    try {
      const updated = await post("/me/subscription", { plus });
      if (updated?.id) setUser(storeAccount(updated));
    } catch (error) {
      if (error.status && error.status !== 404) {
        saveStorage(`mixora-ui:plus:${user.id}`, user.plus);
        setUser(storeAccount(user));
        toast(error.message);
      }
    }
  };
  const logout = async () => {
    const id = user?.id;
    try {
      await post("/auth/logout");
      if (id) {
        saveStorage(
          "mixora-ui:accounts",
          dropAccountToken(readStorage("mixora-ui:accounts", []), id),
        );
      }
      audioRef.current?.pause();
      waveGeneration.current++;
      waveRound.current = 0;
      waveFetching.current = false;
      waveSession.current = "";
      waveTrackSessions.current.clear();
      waveRestoredRef.current = "";
      setWaveModelVersion("rules-v0");
      setWaveActive(false);
      setWaveBusy(false);
      preferenceStateRef.current = {
        userId: "",
        loaded: false,
        settled: false,
        values: [],
      };
      historyStateRef.current = {
        userId: "",
        loaded: false,
        entries: [],
        generation: 0,
      };
      historyClearingRef.current = false;
      playlistStateRef.current = {
        userId: "",
        loaded: false,
        values: [],
      };
      setUser(null);
      setQueue([]);
      setIndex(-1);
      toast("Вы вышли из аккаунта");
    } catch (e) {
      toast(e.message);
    }
  };
  latest.current = {
    next,
    remember,
    current,
    play,
    autoplay: settings.autoplay,
  };
  useEffect(() => {
    let active = true;
    api("/me")
      .then(async (u) => {
        if (!active) return;
        let profile = profileOf(u);
        try {
          const session = await api("/auth/session");
          if (!active) return;
          profile = storeAccount(session, session.token);
        } catch {
          profile = storeAccount(profile);
        }
        if (active) setUser(profile);
      })
      .catch(() => {})
      .finally(() => {
        if (active) setSessionReady(true);
      });
    return () => {
      active = false;
    };
  }, []);
  useEffect(() => {
    if (!sessionReady || !user?.id) {
      waveGeneration.current++;
      waveRound.current = 0;
      waveFetching.current = false;
      waveSession.current = "";
      waveTrackSessions.current.clear();
      waveRestoredRef.current = "";
      setWaveModelVersion("rules-v0");
      setWaveActive(false);
      setWaveBusy(false);
      preferenceStateRef.current = {
        userId: "",
        loaded: false,
        settled: false,
        values: [],
      };
      historyStateRef.current = {
        userId: "",
        loaded: false,
        entries: [],
        generation: 0,
      };
      historyClearingRef.current = false;
      playlistStateRef.current = {
        userId: "",
        loaded: false,
        values: [],
      };
      return;
    }
    const userId = user.id;
    waveGeneration.current++;
    waveRound.current = 0;
    waveFetching.current = false;
    waveSession.current = "";
    waveTrackSessions.current.clear();
    setWaveModelVersion("rules-v0");
    setWaveActive(false);
    setWaveBusy(false);
    preferenceStateRef.current = {
      userId,
      loaded: false,
      settled: false,
      values: [],
    };
    historyStateRef.current = {
      userId,
      loaded: false,
      entries: [],
      generation: 0,
    };
    historyClearingRef.current = false;
    playlistStateRef.current = {
      userId,
      loaded: false,
      values: [],
    };
    void loadTrackPreferences(userId);
    void loadHistory(userId);
    void loadPlaylists(userId);
  }, [sessionReady, user?.id]);
  useEffect(() => {
    if (!sessionReady || !user) return;
    let ignore = false;
    api("/library")
      .then((remote) => {
        if (ignore || saveTimer.current || userRef.current?.id !== user.id)
          return;
        const local = loadLibrary(storageKey);
        const remoteLibrary = withPins({
          ...emptyLibrary(),
          ...(remote || {}),
        });
        const remoteSnapshot = withoutDedicatedStateSnapshot(remoteLibrary);
        const useRemote = libraryCount(remoteSnapshot) > 0;
        const base = useRemote
          ? {
              ...remoteSnapshot,
              // Until the dedicated state arrives, never replace local values
              // with the old library snapshot's likes/dislikes/history fields.
              likes: local.likes,
              dislikes: local.dislikes,
              history: local.history,
              playlists: local.playlists,
            }
          : local;
        const preferenceState = preferenceStateRef.current;
        const pendingPreferences = pendingTrackPreferencesFor(
          user.id,
          preferenceState.values,
        );
        let next =
          preferenceState.userId === user.id && preferenceState.loaded
            ? mergeTrackPreferences(base, preferenceState.values, [
                ...pendingPreferences.queue,
                ...pendingPreferences.backfill,
              ])
            : base;
        const historyState = historyStateRef.current;
        if (historyState.userId === user.id && historyState.loaded) {
          next = mergeHistory(
            next,
            historyState.entries,
            pendingHistoryRecords(user.id),
          );
        }
        const playlistState = playlistStateRef.current;
        if (playlistState.userId === user.id && playlistState.loaded) {
          next = mergePlaylists(
            next,
            playlistState.values,
            pendingPlaylistMutations(user.id),
          );
        }
        setStored({ key: storageKey, value: next });
        saveStorage(storageKey, next);
        if (
          !useRemote &&
          libraryCount(withoutDedicatedStateSnapshot(local)) > 0
        ) {
          put("/library", transitionSafeLibraryPayload(local)).catch(() => {});
        }
      })
      .catch(() => {});
    return () => {
      ignore = true;
    };
  }, [sessionReady, user?.id]);
  useEffect(() => {
    if (!sessionReady || !user?.id) return;
    const userId = user.id;
    const flush = () => {
      void flushEvents(userId);
      // Re-read server-owned state after a reconnect before retrying
      // browser-local mutations. This keeps remote preferences and history
      // authoritative while retaining unsent records as an overlay.
      void loadTrackPreferences(userId);
      void loadHistory(userId);
      void loadPlaylists(userId);
    };
    flush();
    window.addEventListener("online", flush);
    return () => window.removeEventListener("online", flush);
  }, [sessionReady, user?.id]);
  useEffect(() => {
    if (!sessionReady) return;
    const id = user?.id || "guest";
    if (restoredRef.current === id) return;
    restoredRef.current = id;
    const saved = readStorage(`mixora-ui:player:${id}`, null);
    if (!saved?.track?.id) return;
    const list =
      Array.isArray(saved.queue) && saved.queue.length
        ? saved.queue
        : [saved.track];
    resumeRef.current = { position: Number(saved.position) || 0 };
    setQueue(list);
    setIndex(Math.max(0, Math.min(saved.index || 0, list.length - 1)));
  }, [sessionReady, user?.id]);
  useEffect(() => {
    if (!sessionReady || !user?.id) return;
    const userId = user.id;
    if (waveRestoredRef.current === userId) return;
    waveRestoredRef.current = userId;

    const player = readStorage(`mixora-ui:player:${userId}`, null);
    const restoredQueue =
      Array.isArray(player?.queue) && player.queue.length
        ? player.queue
        : player?.track
          ? [player.track]
          : [];
    const restored = restoreWaveSession(
      readStorage(waveSessionStorageKey(userId), null),
      restoredQueue,
    );
    if (!restored) return;

    waveRound.current = restored.round;
    waveSession.current = [...restored.trackSessions.values()].at(-1) || "";
    waveTrackSessions.current = restored.trackSessions;
    setWaveModelVersion(restored.modelVersion);
    setWaveContext(restored.context);
    setSettings({ wave: restored.preferences });
    setWaveActive(true);
  }, [sessionReady, user?.id]);
  useEffect(() => {
    if (!sessionReady || !user?.id || !waveActive) return;
    saveStorage(
      waveSessionStorageKey(user.id),
      createWaveSessionSnapshot({
        modelVersion: waveModelVersion,
        preferences: settings.wave,
        context: waveContext,
        round: waveRound.current,
        trackSessions: waveTrackSessions.current,
      }),
    );
  }, [
    sessionReady,
    user?.id,
    waveActive,
    waveModelVersion,
    settings.wave,
    waveContext,
    queue.length,
    index,
  ]);
  useEffect(() => {
    if (!sessionReady || !current) return;
    const id = user?.id || "guest";
    const timer = setTimeout(() => {
      saveStorage(
        `mixora-ui:player:${id}`,
        playerStorageSnapshot(
          current,
          index,
          audioRef.current?.currentTime || 0,
          queue,
        ),
      );
    }, 500);
    return () => clearTimeout(timer);
  }, [
    sessionReady,
    user?.id,
    current?.id,
    index,
    queue.length,
    Math.floor(position),
  ]);
  useEffect(() => {
    if (user && pending.current) {
      const request = pending.current;
      pending.current = null;
      if (request.wave)
        startWave(request.wave.context, request.wave.preferences);
      else play(request.track, request.list);
    }
  }, [user]);
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(""), 4500);
    return () => clearTimeout(t);
  }, [notice]);
  useEffect(() => {
    document.documentElement.dataset.theme = settings.theme;
    document.documentElement.classList.toggle(
      "ym-light-theme",
      settings.theme === "light",
    );
    document.documentElement.classList.toggle(
      "ym-dark-theme",
      settings.theme !== "light",
    );
    document.documentElement.dataset.motion = settings.animation ? "on" : "off";
    if (audioRef.current) {
      audioRef.current.volume = settings.volume;
      audioRef.current.playbackRate = settings.playbackRate || 1;
    }
  }, [settings]);
  useEffect(() => {
    const a = new Audio();
    a.preload = "metadata";
    audioRef.current = a;
    a.volume = settings.volume;
    const events = {
      timeupdate: () => {
        setPosition(a.currentTime);
        const listenedEnough =
          a.currentTime >= 30 ||
          (Number.isFinite(a.duration) &&
            a.duration > 0 &&
            a.currentTime >= a.duration * 0.5);
        if (!heard.current && listenedEnough && latest.current.current) {
          heard.current = true;
          latest.current.remember(latest.current.current);
        }
      },
      durationchange: () =>
        setLength(Number.isFinite(a.duration) ? a.duration : 0),
      play: () => {
        setPlaying(true);
        setPlaybackError("");
        if (latest.current.current) recordEvent("play", latest.current.current);
      },
      pause: () => setPlaying(false),
      waiting: () => setLoading(true),
      playing: () => setLoading(false),
      ended: () => {
        if (latest.current.current)
          recordEvent("complete", latest.current.current, {
            position_ms: Math.round((a.currentTime || 0) * 1000),
            duration_ms: Math.round((a.duration || 0) * 1000),
          });
        if (latest.current.autoplay) latest.current.next(true);
        else setPlaying(false);
      },
      error: () => {
        if (a.src) {
          setLoading(false);
          setPlaying(false);
          setPlaybackError(
            "Не удалось загрузить аудио. Попробуйте другой трек.",
          );
        }
      },
    };
    Object.entries(events).forEach(([name, fn]) =>
      a.addEventListener(name, fn),
    );
    return () => {
      a.pause();
      a.removeAttribute("src");
      a.load();
      hlsRef.current?.destroy();
      Object.entries(events).forEach(([name, fn]) =>
        a.removeEventListener(name, fn),
      );
    };
  }, []);
  const beginPlayback = (audio) => {
    const resume = resumeRef.current;
    if (resume) {
      const place = () => {
        if (Number.isFinite(resume.position))
          audio.currentTime = resume.position;
        setPosition(resume.position || 0);
        setLoading(false);
        setPlaying(false);
      };
      if (audio.readyState >= 1) place();
      else audio.addEventListener("loadedmetadata", place, { once: true });
      resumeRef.current = null;
      return;
    }
    audio.play().catch(() => {
      setLoading(false);
      setPlaybackError("Нажмите «Воспроизвести», чтобы продолжить.");
    });
  };
  useEffect(() => {
    const a = audioRef.current;
    if (!current || !a) return;
    const controller = new AbortController();
    let cancelled = false;
    a.pause();
    a.removeAttribute("src");
    a.load();
    hlsRef.current?.destroy();
    hlsRef.current = null;
    heard.current = false;
    setPosition(0);
    setLength(0);
    setLoading(true);
    setPlaybackError("");
    (async () => {
      try {
        const playback = await getTrackPlayback(current, controller.signal);
        if (cancelled) return;
        if (playback.format === "hls") {
          const { default: Hls } = await import("hls.js");
          if (cancelled) return;
          if (Hls.isSupported()) {
            const hls = new Hls();
            hlsRef.current = hls;
            hls.loadSource(playback.url);
            hls.attachMedia(a);
            hls.on(Hls.Events.MANIFEST_PARSED, () => {
              if (cancelled) return;
              applyQuality(hls, settingsRef.current.quality);
              beginPlayback(a);
            });
            hls.on(Hls.Events.ERROR, (_, data) => {
              if (data.fatal && !cancelled) {
                setLoading(false);
                setPlaying(false);
                setPlaybackError("Поток недоступен. Выберите трек повторно.");
                hls.destroy();
              }
            });
          } else if (a.canPlayType("application/vnd.apple.mpegurl")) {
            a.src = playback.url;
            beginPlayback(a);
          } else {
            throw Error("Этот браузер не поддерживает воспроизведение HLS.");
          }
        } else {
          a.src = playback.url;
          beginPlayback(a);
        }
        if (!cancelled) {
          if ("mediaSession" in navigator) {
            navigator.mediaSession.metadata = new MediaMetadata({
              title: current.title,
              artist: current.artist,
              artwork: current.artwork ? [{ src: current.artwork }] : [],
            });
          }
        }
      } catch (e) {
        if (!cancelled && e.name !== "AbortError") {
          setLoading(false);
          setPlaybackError(
            e.name === "NotSupportedError"
              ? "Не удалось открыть аудиопоток. Выберите другой трек."
              : e.message,
          );
          if (e.status === 401) setAuthOpen(true);
        }
      }
    })();
    return () => {
      cancelled = true;
      controller.abort();
      a.pause();
      hlsRef.current?.destroy();
      hlsRef.current = null;
    };
  }, [current?.id, current?.source, playbackAttempt]);
  useEffect(() => {
    applyQuality(hlsRef.current, settings.quality);
  }, [settings.quality]);
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    if (!settings.equalizer) {
      if (eqRef.current) eqRef.current.filter.gain.value = 0;
      return;
    }
    try {
      if (!eqRef.current) {
        const context = new AudioContext();
        const source = context.createMediaElementSource(audio);
        const filter = context.createBiquadFilter();
        filter.type = "peaking";
        filter.frequency.value = 1200;
        filter.Q.value = 0.7;
        source.connect(filter);
        filter.connect(context.destination);
        eqRef.current = { context, filter };
      }
      eqRef.current.filter.gain.value = 5;
      eqRef.current.context.resume();
    } catch {
      /* The element can be routed only once. */
    }
  }, [settings.equalizer]);
  const socketRef = useRef(null);
  const suppressSync = useRef(0);
  useEffect(() => {
    const ws = socketRef.current;
    if (!user || !current || !ws || ws.readyState !== 1 || suppressSync.current)
      return;
    ws.send(
      JSON.stringify(
        playbackSnapshot(
          current,
          playing,
          audioRef.current?.currentTime || 0,
          queue,
        ),
      ),
    );
  }, [user?.id, current?.id, playing, index, queue.length]);
  useEffect(() => {
    if (!user) return;
    let stopped = false;
    let retry = 0;
    let ws;
    const connect = () => {
      const url = `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api/v1/playback/ws`;
      ws = new WebSocket(url);
      socketRef.current = ws;
      ws.onmessage = (event) => {
        let msg;
        try {
          msg = JSON.parse(event.data);
        } catch {
          return;
        }
        if (msg?.type !== "state" || !msg.track?.id) return;
        suppressSync.current += 1;
        const here = latest.current.current;
        if (!here || trackKey(msg.track) !== trackKey(here)) {
          latest.current.play(
            msg.track,
            msg.queue?.length ? msg.queue : [msg.track],
          );
        }
        const audio = audioRef.current;
        if (
          audio &&
          Number.isFinite(msg.position) &&
          Math.abs((audio.currentTime || 0) - msg.position) > 2
        ) {
          audio.currentTime = msg.position;
        }
        if (msg.playing) audio?.play().catch(() => {});
        else audio?.pause();
        setTimeout(() => {
          suppressSync.current = Math.max(0, suppressSync.current - 1);
        }, 500);
      };
      ws.onclose = () => {
        if (socketRef.current === ws) socketRef.current = null;
        if (!stopped) retry = window.setTimeout(connect, 2000);
      };
    };
    connect();
    return () => {
      stopped = true;
      window.clearTimeout(retry);
      ws?.close();
      if (socketRef.current === ws) socketRef.current = null;
    };
  }, [user?.id]);
  useEffect(() => {
    if (!("mediaSession" in navigator)) return;
    const actions = {
      play: () => audioRef.current?.play().catch(() => {}),
      pause: () => audioRef.current?.pause(),
      nexttrack: () => latest.current.next(),
      previoustrack: previous,
      seekto: (e) => seek(e.seekTime),
    };
    Object.entries(actions).forEach(([name, fn]) => {
      try {
        navigator.mediaSession.setActionHandler(name, fn);
      } catch {}
    });
  }, [index, queue.length]);
  const toggleShuffle = () => {
    if (!shuffled && current) {
      setQueue((q) => shuffleTracks(q, index));
      setIndex(0);
    }
    setShuffled((s) => !s);
  };
  return (
    <Context.Provider
      value={{
        user,
        waveActive,
        waveBusy,
        waveModelVersion,
        waveContext,
        wavePreferences,
        waveSettingsOpen,
        setWaveSettingsOpen,
        startWave,
        recordSearch,
        dislike,
        clearTrackPreference,
        toggleSaved,
        editPlaylist,
        removeQueue,
        moveQueue,
        sessionReady,
        login,
        resume,
        logout,
        setPlus,
        authOpen,
        setAuthOpen,
        authIntent,
        setAuthIntent,
        notice,
        toast,
        panel,
        setPanel,
        settings,
        setSettings,
        catalog,
        setCatalog,
        library,
        updateLibrary,
        clearHistory,
        play,
        current,
        playing,
        loading,
        position,
        length,
        playbackError,
        toggle,
        next,
        previous,
        seek,
        queue,
        setQueue,
        index,
        setIndex,
        repeat,
        setRepeat,
        shuffled,
        toggleShuffle,
        toggleLike,
        addQueue,
        createPlaylist,
        addToPlaylist,
        removeFromPlaylist,
        deletePlaylist,
        togglePin,
        likeOwnPlaylist,
      }}
    >
      {children}
    </Context.Provider>
  );
}
