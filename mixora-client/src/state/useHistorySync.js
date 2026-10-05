import { useRef } from "react";
import { api, put } from "../lib/api.js";
import {
  acknowledgeHistoryRecords,
  bindHistoryGeneration,
  historyBatch,
  historyEntries,
  historyQueueKey,
  historyRequest,
  historySnapshot,
  mergeHistory,
  mergeHistoryEntries,
  normalizeHistoryEntry,
} from "../lib/history.js";
import { readStorage, saveStorage } from "../lib/library.js";
import { loadLibrary, storedList } from "./libraryStorage.js";

export const pendingHistoryRecords = (userId) =>
  storedList(historyQueueKey(userId));

// Keeps history's clear-generation fence, local queue and remote snapshot in
// one place. The AppProvider still coordinates a user change, player events
// and the public context surface.
export function useHistorySync({ userRef, setAuthOpen, setStored, toast }) {
  const historyFlushRef = useRef(false);
  const historyClearingRef = useRef(false);
  const historyLoadRef = useRef("");
  const historyStateRef = useRef({
    userId: "",
    loaded: false,
    entries: [],
    generation: 0,
  });

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

  return {
    applyHistoryState,
    clearHistory,
    flushHistory,
    historyClearingRef,
    historyStateRef,
    loadHistory,
  };
}
