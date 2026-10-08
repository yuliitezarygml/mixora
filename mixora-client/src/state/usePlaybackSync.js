import { useEffect, useRef } from "react";
import {
  decodePlaybackState,
  playbackSnapshot,
  playbackSocketURL,
} from "../lib/playbackSync.js";

const RECONNECT_DELAY_MS = 2000;
const REMOTE_RELEASE_DELAY_MS = 500;

// Keeps cross-window playback state isolated from the player itself. Audio,
// queue and Media Session ownership stay in AppContext; this hook only sends
// snapshots and applies valid remote snapshots to their latest callbacks.
export function usePlaybackSync({
  userId,
  current,
  playing,
  index,
  queue,
  audioRef,
  applyRemoteState,
}) {
  const socketRef = useRef(null);
  const suppressSyncRef = useRef(0);
  const latestRef = useRef({});
  latestRef.current = { current, playing, queue, applyRemoteState };

  useEffect(() => {
    const socket = socketRef.current;
    const latest = latestRef.current;
    if (
      !userId ||
      !latest.current ||
      !socket ||
      socket.readyState !== 1 ||
      suppressSyncRef.current
    ) {
      return;
    }
    socket.send(
      JSON.stringify(
        playbackSnapshot(
          latest.current,
          latest.playing,
          audioRef.current?.currentTime || 0,
          latest.queue,
        ),
      ),
    );
  }, [userId, current?.id, current?.source, playing, index, queue, audioRef]);

  useEffect(() => {
    if (!userId) return undefined;

    let stopped = false;
    let reconnectTimer = 0;
    let latestSocket = null;
    const releaseTimers = new Set();

    const scheduleReconnect = () => {
      if (stopped || reconnectTimer) return;
      reconnectTimer = window.setTimeout(() => {
        reconnectTimer = 0;
        connect();
      }, RECONNECT_DELAY_MS);
    };

    const scheduleRelease = () => {
      const timer = window.setTimeout(() => {
        releaseTimers.delete(timer);
        if (stopped) return;
        suppressSyncRef.current = Math.max(0, suppressSyncRef.current - 1);
      }, REMOTE_RELEASE_DELAY_MS);
      releaseTimers.add(timer);
    };

    const connect = () => {
      if (stopped) return;
      let socket;
      try {
        socket = new WebSocket(playbackSocketURL(window.location));
      } catch {
        scheduleReconnect();
        return;
      }
      latestSocket = socket;
      socketRef.current = socket;
      socket.onmessage = (event) => {
        if (stopped || socketRef.current !== socket) return;
        const message = decodePlaybackState(event.data);
        if (!message) return;

        suppressSyncRef.current += 1;
        latestRef.current.applyRemoteState(message);
        scheduleRelease();
      };
      socket.onclose = () => {
        if (socketRef.current === socket) socketRef.current = null;
        if (!stopped) scheduleReconnect();
      };
    };

    connect();
    return () => {
      stopped = true;
      if (reconnectTimer) window.clearTimeout(reconnectTimer);
      reconnectTimer = 0;
      for (const timer of releaseTimers) window.clearTimeout(timer);
      releaseTimers.clear();
      suppressSyncRef.current = 0;
      latestSocket?.close();
      if (socketRef.current === latestSocket) socketRef.current = null;
    };
  }, [userId, audioRef]);
}
