import { useEffect, useRef, useState } from "react";
import { AppContext as Context } from "./context.js";
import { api, getTrackPlayback, post, put } from "../lib/api.js";
import {
  readStorage,
  saveStorage,
  trackKey,
  uniqueTracks,
  shuffleTracks,
  libraryPayload,
  libraryCount,
} from "../lib/library.js";
import { dropAccountToken, rememberAccount } from "../lib/accounts.js";
import { playbackSnapshot } from "../lib/playbackSync.js";
import {
  acknowledgeEvents,
  enqueueEvent,
  eventBatch,
  eventQueueKey,
} from "../lib/eventQueue.js";
import { buildWave, defaultWave } from "../lib/wave.js";
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
function withPins(library) {
  if (Array.isArray(library.pins)) return library;
  return {
    ...library,
    pins: (library.playlists || []).map((playlist) => playlist.id),
  };
}
const loadLibrary = (key) =>
  withPins({ ...emptyLibrary(), ...readStorage(key, {}) });
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
    [waveContext, setWaveContext] = useState(null),
    [waveSettingsOpen, setWaveSettingsOpen] = useState(false);
  const waveGeneration = useRef(0),
    waveRound = useRef(0),
    waveFetching = useRef(false),
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
    eventFlushRef = useRef(false),
    eqRef = useRef(null),
    settingsRef = useRef(settings);
  settingsRef.current = settings;
  const current = queue[index] || null;
  const updateLibrary = (fn) => {
    setStored((previous) => {
      const base =
        previous.key === storageKey ? previous.value : loadLibrary(storageKey);
      const next = fn(base);
      saveStorage(storageKey, next);
      if (userRef.current) {
        clearTimeout(saveTimer.current);
        saveTimer.current = setTimeout(() => {
          saveTimer.current = 0;
          put("/library", libraryPayload(next)).catch(() => {});
        }, 600);
      }
      return { key: storageKey, value: next };
    });
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
  const recordEvent = (type, track, extra = {}) => {
    const userId = userRef.current?.id;
    if (!userId || !track?.id) return;
    const event = {
      idempotency_key: crypto.randomUUID(),
      type,
      track_source: track.source || "music",
      track_id: String(track.id),
      occurred_at: new Date().toISOString(),
      ...extra,
    };
    const key = eventQueueKey(userId);
    saveStorage(key, enqueueEvent(readStorage(key, []), event));
    void flushEvents(userId);
  };
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
    updateLibrary((s) => ({
      ...s,
      history: uniqueTracks([track, ...s.history]).slice(0, 100),
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
  const next = async (auto = false) => {
    if (!queue.length) return;
    if (auto && repeat === "one") {
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
        const tracks = await loadWave(
          wavePreferences,
          waveContext,
          queue.slice(-12),
        );
        if (generation !== waveGeneration.current) return;
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
    const seeds = uniqueTracks([
      ...catalog,
      ...library.likes,
      ...library.history,
    ]).slice(0, 40);
    const local = () =>
      buildWave(seeds, library, preferences, {
        explicit: settings.explicit,
        exclude,
      });
    try {
      const data = await post("/wave", {
        preferences,
        context: context || {},
        round: waveRound.current++,
        explicit: settings.explicit,
        exclude,
        likes: library.likes.slice(0, 40),
        history: library.history.slice(0, 40),
        dislikes: library.dislikes.slice(0, 80),
        seeds,
      });
      const tracks = Array.isArray(data?.tracks) ? data.tracks : [];
      if (tracks.length)
        setCatalog((previous) => uniqueTracks([...previous, ...tracks]));
      return tracks;
    } catch (error) {
      if (error.status !== 503) toast(error.message);
      return local();
    }
  };
  const startWave = async (context = null, preferences = wavePreferences) => {
    if (!user) {
      pending.current = { wave: { context, preferences } };
      setAuthOpen(true);
      return;
    }
    const generation = ++waveGeneration.current;
    setWaveBusy(true);
    setWaveContext(context);
    setSettings({ wave: preferences });
    try {
      const tracks = await loadWave(preferences, context);
      if (generation !== waveGeneration.current) return;
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
  const dislike = (track) => {
    recordEvent("dislike", track);
    updateLibrary((s) => ({
      ...s,
      dislikes: uniqueTracks([track, ...s.dislikes]),
      likes: s.likes.filter((t) => trackKey(t) !== trackKey(track)),
    }));
    if (current && trackKey(current) === trackKey(track)) next();
    toast("Больше не будем предлагать этот трек в Моей волне");
  };
  const toggleSaved = (field, entity) =>
    updateLibrary((s) => ({
      ...s,
      [field]: s[field].some((x) => x.id === entity.id)
        ? s[field].filter((x) => x.id !== entity.id)
        : [entity, ...s[field]],
    }));
  const editPlaylist = (id, patch) =>
    updateLibrary((s) => ({
      ...s,
      playlists: s.playlists.map((p) =>
        p.id === id ? { ...p, ...patch, id: p.id } : p,
      ),
    }));
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
      audioRef.current.currentTime = value;
      setPosition(value);
    }
  };
  const toggleLike = (track) => {
    const adding = !library.likes.some(
      (saved) => trackKey(saved) === trackKey(track),
    );
    if (adding) recordEvent("like", track);
    updateLibrary((s) => {
      const exists = s.likes.some((t) => trackKey(t) === trackKey(track));
      return {
        ...s,
        likes: exists
          ? s.likes.filter((t) => trackKey(t) !== trackKey(track))
          : [track, ...s.likes],
      };
    });
  };
  const addQueue = (track) => {
    setQueue((q) => [...q, track]);
    toast("Трек добавлен в очередь");
  };
  const createPlaylist = (name) => {
    const p = {
      id: crypto.randomUUID(),
      name: name.trim(),
      tracks: [],
      createdAt: Date.now(),
    };
    updateLibrary((s) => ({
      ...s,
      playlists: [p, ...s.playlists],
      pins: [p.id, ...(s.pins || [])],
    }));
    return p;
  };
  const addToPlaylist = (id, track) => {
    recordEvent("add_to_playlist", track, { context: { playlist_id: id } });
    updateLibrary((s) => ({
      ...s,
      playlists: s.playlists.map((p) =>
        p.id === id ? { ...p, tracks: uniqueTracks([...p.tracks, track]) } : p,
      ),
    }));
    toast("Трек добавлен в плейлист");
  };
  const removeFromPlaylist = (id, track) =>
    updateLibrary((s) => ({
      ...s,
      playlists: s.playlists.map((p) =>
        p.id === id
          ? {
              ...p,
              tracks: p.tracks.filter((t) => trackKey(t) !== trackKey(track)),
            }
          : p,
      ),
    }));
  const deletePlaylist = (id) =>
    updateLibrary((s) => ({
      ...s,
      pins: (s.pins || []).filter((pin) => pin !== id),
      playlists: s.playlists.filter((p) => p.id !== id),
    }));
  const togglePin = (id) =>
    updateLibrary((s) => {
      const pins = s.pins || [];
      return {
        ...s,
        pins: pins.includes(id)
          ? pins.filter((pin) => pin !== id)
          : [id, ...pins],
      };
    });
  const likeOwnPlaylist = (id) =>
    updateLibrary((s) => ({
      ...s,
      playlists: s.playlists.map((playlist) =>
        playlist.id === id ? { ...playlist, liked: !playlist.liked } : playlist,
      ),
    }));
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
      setWaveActive(false);
      setWaveBusy(false);
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
    if (!sessionReady || !user) return;
    let ignore = false;
    api("/library")
      .then((remote) => {
        if (ignore || saveTimer.current) return;
        const local = loadLibrary(storageKey);
        const remoteLibrary = withPins({
          ...emptyLibrary(),
          ...(remote || {}),
        });
        const useRemote = libraryCount(remoteLibrary) > 0;
        const next = useRemote ? remoteLibrary : local;
        setStored({ key: storageKey, value: next });
        saveStorage(storageKey, next);
        if (!useRemote && libraryCount(local) > 0) {
          put("/library", libraryPayload(local)).catch(() => {});
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
    const flush = () => void flushEvents(userId);
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
    if (!sessionReady || !current) return;
    const id = user?.id || "guest";
    const timer = setTimeout(() => {
      saveStorage(`mixora-ui:player:${id}`, {
        track: current,
        queue: queue.slice(0, 30),
        index,
        position: Math.round((audioRef.current?.currentTime || 0) * 10) / 10,
      });
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
        const playback = await getTrackPlayback(current.id, controller.signal);
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
        waveContext,
        wavePreferences,
        waveSettingsOpen,
        setWaveSettingsOpen,
        startWave,
        dislike,
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
