import { useEffect, useRef, useState } from "react";
import { AppContext as Context } from "./context.js";
import {
  emptyLibrary,
  loadLibrary,
  pendingTrackPreferencesFor,
  transitionSafeLibraryPayload,
  useLibraryStorage,
  withPins,
  withoutDedicatedStateSnapshot,
} from "./libraryStorage.js";
import { useListeningEvents } from "./useListeningEvents.js";
import { pendingHistoryRecords, useHistorySync } from "./useHistorySync.js";
import { usePlaybackSync } from "./usePlaybackSync.js";
import { usePlayerPersistence } from "./usePlayerPersistence.js";
import { useRecommendedPlaylists } from "./useRecommendedPlaylists.js";
import { filterRecommendedTracks } from "../lib/recommendedPlaylists.js";
import {
  pendingPlaylistMutations,
  usePlaylistSync,
} from "./usePlaylistSync.js";
import { useTrackPreferenceSync } from "./useTrackPreferenceSync.js";
import { api, getTrackPlayback, post, put } from "../lib/api.js";
import {
  readStorage,
  saveStorage,
  entityKey,
  trackKey,
  uniqueTracks,
  shuffleTracks,
  libraryCount,
} from "../lib/library.js";
import { dropAccountToken, rememberAccount } from "../lib/accounts.js";
import {
  shouldApplyRemotePosition,
  shouldResolvePlayback,
} from "../lib/playbackSync.js";
import {
  createHistoryRecord,
  enqueueHistoryRecord,
  historyQueueKey,
  mergeHistory,
} from "../lib/history.js";
import {
  createPlaylistDeleteMutation,
  createPlaylistMutation,
  enqueuePlaylistMutation,
  mergePlaylists,
  playlistQueueKey,
} from "../lib/playlists.js";
import { buildWave, defaultWave, waveExplanation } from "../lib/wave.js";
import { buildWaveRequest, decodeWaveResponse } from "../lib/waveRequest.js";
import {
  createWaveSessionSnapshot,
  restoreWaveSession,
  updateWaveTrackSessions,
  waveSessionStorageKey,
} from "../lib/waveSession.js";
import {
  createTrackPreferenceMutation,
  enqueueTrackPreference,
  mergeTrackPreferences,
  preserveDisplacedTrackPreferenceMigrations,
  trackPreferenceBackfillKey,
  trackPreferenceQueueKey,
} from "../lib/trackPreferences.js";
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
    [online, setOnline] = useState(
      () => typeof navigator === "undefined" || navigator.onLine !== false,
    ),
    [panel, setPanel] = useState(null),
    [queue, setQueue] = useState([]),
    [index, setIndex] = useState(-1),
    [playerOwner, setPlayerOwner] = useState("");
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
    [waveFallback, setWaveFallback] = useState(false),
    [waveContext, setWaveContext] = useState(null),
    [waveSettingsOpen, setWaveSettingsOpen] = useState(false);
  const userRef = useRef(null);
  userRef.current = user;
  const { library, saveTimer, setStored, storageKey, updateLibrary } =
    useLibraryStorage(user, userRef);
  const waveGeneration = useRef(0),
    waveRound = useRef(0),
    waveFetching = useRef(false),
    waveSession = useRef(""),
    waveTrackSessions = useRef(new Map()),
    heard = useRef(false);
  const wavePreferences = { ...defaultWave, ...settings.wave };
  const recommendations = useRecommendedPlaylists({
    userId: user?.id,
    library,
    catalog,
    preferences: wavePreferences,
    explicit: settings.explicit,
  });
  const audioRef = useRef(null),
    hlsRef = useRef(null),
    pending = useRef(null),
    latest = useRef({}),
    resumeRef = useRef(null),
    restoredRef = useRef(""),
    waveRestoredRef = useRef(""),
    eqRef = useRef(null),
    settingsRef = useRef(settings);
  settingsRef.current = settings;
  const toast = (message) => setNotice(message);
  const {
    applyTrackPreferenceState,
    flushTrackPreferences,
    loadTrackPreferences,
    preferenceStateRef,
  } = useTrackPreferenceSync(userRef, setStored);
  const {
    applyHistoryState,
    clearHistory,
    flushHistory,
    historyClearingRef,
    historyStateRef,
    loadHistory,
  } = useHistorySync({ userRef, setAuthOpen, setStored, toast });
  const {
    applyPlaylistState,
    flushPlaylists,
    loadPlaylists,
    playlistStateRef,
  } = usePlaylistSync({ userRef, setStored, toast });
  const {
    enqueueListeningEvents,
    flushEvents,
    recordEvent,
    recordSearch,
    seekListeningEvent,
  } = useListeningEvents(userRef, waveTrackSessions);
  const current = queue[index] || null;
  useEffect(() => {
    if (typeof window === "undefined") return undefined;
    const updateConnection = () =>
      setOnline(typeof navigator === "undefined" || navigator.onLine !== false);
    updateConnection();
    window.addEventListener("online", updateConnection);
    window.addEventListener("offline", updateConnection);
    return () => {
      window.removeEventListener("online", updateConnection);
      window.removeEventListener("offline", updateConnection);
    };
  }, []);
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
      setWaveFallback(false);
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
      if (
        shouldResolvePlayback({
          hasSource: Boolean(audioRef.current?.src),
          mediaError: audioRef.current?.error,
        })
      ) {
        setPlaybackAttempt((attempt) => attempt + 1);
        return;
      }
      setPlaybackError("");
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
    if (
      shouldResolvePlayback({
        hasSource: Boolean(a.src),
        mediaError: a.error,
      })
    ) {
      setPlaybackAttempt((attempt) => attempt + 1);
      return;
    }
    setPlaybackError("");
    if (a.paused) {
      a.play().catch(() =>
        setPlaybackError("Не удалось начать воспроизведение."),
      );
    } else a.pause();
  };
  const applyRemotePlayback = (message) => {
    const list =
      Array.isArray(message.queue) && message.queue.length
        ? message.queue
        : [message.track];
    if (!current || trackKey(current) !== trackKey(message.track)) {
      resumeRef.current = {
        trackKey: trackKey(message.track),
        position:
          Number.isFinite(message.position) && message.position >= 0
            ? message.position
            : 0,
        playing: message.playing === true,
      };
      play(message.track, list);
      return;
    }
    const allowed = list.filter(
      (track) =>
        track &&
        track.access !== "blocked" &&
        (settings.explicit || !track.explicit),
    );
    const remoteIndex = allowed.findIndex(
      (track) => trackKey(track) === trackKey(current),
    );
    if (remoteIndex >= 0) {
      setQueue(allowed);
      setIndex(remoteIndex);
    }
    const audio = audioRef.current;
    if (
      audio &&
      shouldApplyRemotePosition(audio.currentTime || 0, message.position)
    ) {
      audio.currentTime = message.position;
      setPosition(message.position);
    }
    if (message.playing) audio?.play().catch(() => {});
    else audio?.pause();
  };
  const retryPlayback = () => {
    if (!current) return;
    setPlaybackError("");
    // Playback URLs from external sources expire. Incrementing this attempt
    // deliberately re-enters the resolver effect below instead of replaying
    // an old signed CDN URL.
    setPlaybackAttempt((attempt) => attempt + 1);
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
  const rememberRecommendedPlaylist = (playlist) => {
    if (
      !userRef.current ||
      playlist.owner !== userRef.current.id ||
      !playlist.sessionId
    )
      return;
    waveTrackSessions.current = updateWaveTrackSessions(
      waveTrackSessions.current,
      playlist.tracks,
      playlist.sessionId,
    );
  };
  const playRecommendedPlaylist = (playlist, selectedTrack = null) => {
    if (!userRef.current || playlist.owner !== userRef.current.id) return;
    const tracks = filterRecommendedTracks(
      playlist.tracks,
      library,
      settings.explicit,
      playlist.id === "discover",
    );
    const track = selectedTrack
      ? tracks.find((item) => trackKey(item) === trackKey(selectedTrack))
      : tracks[0];
    play(track, tracks);
    if (track) rememberRecommendedPlaylist({ ...playlist, tracks });
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
        fallback: false,
        error: null,
      };
    } catch (error) {
      return {
        tracks: local(),
        sessionId: "",
        modelVersion: "rules-v0",
        fallback: true,
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
    setWaveFallback(Boolean(result.fallback));
    if (result.tracks.length) {
      setCatalog((previous) => uniqueTracks([...previous, ...result.tracks]));
    }
    if (result.fallback) {
      toast(
        result.error?.status === 401
          ? `${result.error.message} Локальная подборка включена временно.`
          : waveExplanation(result.modelVersion, true),
      );
    } else if (result.error?.message) {
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
    setWaveFallback(false);
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
    if (!saveStorage(key, nextQueue)) {
      toast("Не удалось сохранить настройку трека на этом устройстве.");
      return false;
    }
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
    recordEvent("dislike", track);
    if (current && trackKey(current) === trackKey(track)) next(false, true);
    toast("Больше не будем предлагать этот трек в Моей волне");
  };
  const toggleSaved = (field, entity) =>
    updateLibrary((s) => ({
      ...s,
      [field]: s[field].some((item) => entityKey(item) === entityKey(entity))
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
    if (!setTrackPreference(track, adding ? "liked" : "neutral")) return;
    if (adding) recordEvent("like", track);
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
      setWaveFallback(false);
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
      setWaveFallback(false);
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
    setWaveFallback(false);
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
    setPlayerOwner(id);
    if (!saved?.track?.id) {
      resumeRef.current = null;
      setQueue([]);
      setIndex(-1);
      return;
    }
    const list =
      Array.isArray(saved.queue) && saved.queue.length
        ? saved.queue
        : [saved.track];
    const restoredIndex = Math.max(
      0,
      Math.min(saved.index || 0, list.length - 1),
    );
    resumeRef.current = {
      trackKey: trackKey(list[restoredIndex]),
      position: Number(saved.position) || 0,
    };
    setQueue(list);
    setIndex(restoredIndex);
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
    setWaveFallback(false);
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
    queue,
    index,
  ]);
  usePlayerPersistence({
    enabled: sessionReady && playerOwner === (user?.id || "guest"),
    userId: user?.id,
    current,
    index,
    queue,
    position,
    audioRef,
    resumeRef,
  });
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
  const beginPlayback = (audio, isActive) => {
    const resume = resumeRef.current;
    if (resume && resume.trackKey === trackKey(current)) {
      const place = () => {
        if (!isActive()) return;
        if (Number.isFinite(resume.position))
          audio.currentTime = resume.position;
        setPosition(resume.position || 0);
        setLoading(false);
        if (resume.playing) {
          audio.play().catch(() => {
            setPlaybackError("Нажмите «Воспроизвести», чтобы продолжить.");
          });
        } else setPlaying(false);
        if (resumeRef.current === resume) resumeRef.current = null;
      };
      if (audio.readyState >= 1) place();
      else audio.addEventListener("loadedmetadata", place, { once: true });
      return () => audio.removeEventListener("loadedmetadata", place);
    }
    resumeRef.current = null;
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
    let releaseResumeListener;
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
              releaseResumeListener = beginPlayback(a, () => !cancelled);
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
            releaseResumeListener = beginPlayback(a, () => !cancelled);
          } else {
            throw Error("Этот браузер не поддерживает воспроизведение HLS.");
          }
        } else {
          a.src = playback.url;
          releaseResumeListener = beginPlayback(a, () => !cancelled);
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
      releaseResumeListener?.();
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
  usePlaybackSync({
    userId: user?.id,
    current,
    playing,
    index,
    queue,
    audioRef,
    applyRemoteState: applyRemotePlayback,
  });
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
        online,
        waveActive,
        waveBusy,
        waveModelVersion,
        waveFallback,
        waveContext,
        wavePreferences,
        waveSettingsOpen,
        setWaveSettingsOpen,
        startWave,
        recommendations,
        playRecommendedPlaylist,
        rememberRecommendedPlaylist,
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
        retryPlayback,
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
