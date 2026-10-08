import { playbackSocketURL } from "./playbackSync.js";

// The shared UI knows neither Rust IPC nor native cookie storage. Install the
// platform adapter before rendering; web/Electron keep their existing behavior.
let runtime = {};

export function configureClientRuntime(adapter) {
  const previous = runtime;
  runtime = adapter || {};
  return () => {
    runtime = previous;
  };
}

export function clientRequest(url, options) {
  return (runtime.request || globalThis.fetch)(url, options);
}

export function prepareClientPlayback(playback, signal) {
  return runtime.preparePlayback
    ? runtime.preparePlayback(playback, signal)
    : playback;
}

export function createPlaybackSocket(locationLike) {
  return runtime.createPlaybackSocket
    ? runtime.createPlaybackSocket(locationLike)
    : new WebSocket(playbackSocketURL(locationLike));
}
