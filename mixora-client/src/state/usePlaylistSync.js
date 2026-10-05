import { useRef } from "react";
import { api, put } from "../lib/api.js";
import { readStorage, saveStorage } from "../lib/library.js";
import {
  acknowledgePlaylistMutations,
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
import { loadLibrary, storedList } from "./libraryStorage.js";

export const pendingPlaylistMutations = (userId) =>
  storedList(playlistQueueKey(userId));

// Owns the playlist desired-state queue, including revision rebasing and
// legacy migration. UI-facing actions stay in AppContext so its public API is
// unchanged while the state domains are separated incrementally.
export function usePlaylistSync({ userRef, setStored, toast }) {
  const playlistFlushRef = useRef(false);
  const playlistLoadRef = useRef("");
  const playlistStateRef = useRef({
    userId: "",
    loaded: false,
    values: [],
  });

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

  return {
    applyPlaylistState,
    flushPlaylists,
    loadPlaylists,
    playlistStateRef,
  };
}
