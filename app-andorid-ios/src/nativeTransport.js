const abortError = () => new DOMException("Request aborted", "AbortError");

// Cancellation fences late results; Rust's bounded request may still finish.
// No cookie or arbitrary HTTP header crosses this bridge.
function invokeWithAbort(invoke, command, args, signal) {
  if (signal?.aborted) return Promise.reject(abortError());
  return new Promise((resolve, reject) => {
    const abort = () => reject(abortError());
    signal?.addEventListener("abort", abort, { once: true });
    Promise.resolve().then(() => {
      if (signal?.aborted) throw abortError();
      return invoke(command, args);
    }).then(resolve, reject).finally(() => signal?.removeEventListener("abort", abort));
  });
}

export function createNativeTransport(invoke) {
  return {
    async request(path, options = {}) {
      const result = await invokeWithAbort(invoke, "api_request", {
        request: {
          path,
          method: options.method || "GET",
          body: options.body ?? null,
        },
      }, options.signal);
      return new Response(result.status === 204 ? null : result.body, {
        status: result.status,
        headers: { "Content-Type": "application/json" },
      });
    },
    async preparePlayback(playback) {
      if (playback.url.startsWith("/api/")) {
        const error = new Error("Для этого источника ещё подключается нативный плеер. Откройте трек в текущем ПК-клиенте.");
        error.name = "ApiError";
        error.status = 501;
        throw error;
      }
      return playback;
    },
    // A WebView socket cannot access Rust's HttpOnly cookie jar.
    createPlaybackSocket: () => null,
  };
}
