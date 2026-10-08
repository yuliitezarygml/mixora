import { useEffect, useRef } from "react";
import { saveStorage, trackKey } from "../lib/library.js";
import { playerStorageSnapshot } from "../lib/playbackSync.js";

// Queue edits and track changes are durable immediately. Only the frequent
// position updates are delayed, with a final flush on hide/close/unmount.
export function usePlayerPersistence({
  enabled,
  userId,
  current,
  index,
  queue,
  position,
  audioRef,
  resumeRef,
}) {
  const latestRef = useRef(null);
  const savedTrackRef = useRef("");
  const flushRef = useRef(null);
  latestRef.current = { enabled, userId, current, index, queue, position };
  flushRef.current = () => {
    const state = latestRef.current;
    if (!state.enabled || !state.current) return;
    const scope = state.userId || "guest";
    const key = trackKey(state.current);
    const scopedTrack = `${scope}:${key}`;
    const resume = resumeRef.current;
    const nextPosition =
      resume?.trackKey === key
        ? resume.position
        : savedTrackRef.current !== scopedTrack
          ? 0
          : (audioRef.current?.currentTime ?? state.position);
    saveStorage(
      `mixora-ui:player:${scope}`,
      playerStorageSnapshot(
        state.current,
        state.index,
        nextPosition,
        state.queue,
      ),
    );
    savedTrackRef.current = scopedTrack;
  };

  useEffect(() => {
    flushRef.current();
  }, [enabled, userId, current, index, queue]);

  useEffect(() => {
    if (!enabled || !current) return undefined;
    const timer = window.setTimeout(() => flushRef.current(), 500);
    return () => window.clearTimeout(timer);
  }, [enabled, userId, Math.floor(position)]);

  useEffect(() => {
    const flush = () => flushRef.current();
    const onVisibilityChange = () => {
      if (document.visibilityState === "hidden") flush();
    };
    window.addEventListener("pagehide", flush);
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      flush();
      window.removeEventListener("pagehide", flush);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, []);
}
