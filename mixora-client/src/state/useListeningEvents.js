import { useRef } from "react";
import { post } from "../lib/api.js";
import {
  acknowledgeEvents,
  enqueueEvent,
  eventBatch,
  eventQueueKey,
} from "../lib/eventQueue.js";
import {
  seekListeningEvent,
  searchListeningEvents,
  trackListeningEvent,
} from "../lib/listeningEvents.js";
import { trackKey, readStorage, saveStorage } from "../lib/library.js";
import { waveFeedbackPath, withWaveSession } from "../lib/waveFeedback.js";

// Durable listener-event delivery is independent from rendering and playback.
// Keeping it in one hook preserves account fences and idempotency queues while
// allowing AppContext to remain a thin coordinator during the gradual split.
export function useListeningEvents(userRef, waveTrackSessions) {
  const eventFlushRef = useRef(false);

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

  return {
    enqueueListeningEvents,
    flushEvents,
    recordEvent,
    recordSearch,
    seekListeningEvent,
  };
}
