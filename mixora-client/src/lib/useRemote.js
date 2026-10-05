import { useEffect, useRef, useState } from "react";
import { ApiError, networkIsOnline } from "./api.js";

const cache = new Map();
export function useRemote(key, load, enabled = true) {
  const loader = useRef(load);
  loader.current = load;
  const [attempt, setAttempt] = useState(0);
  const [online, setOnline] = useState(networkIsOnline);
  const [state, setState] = useState({
    key: "",
    data: null,
    loading: false,
    error: null,
  });
  useEffect(() => {
    if (typeof window === "undefined") return undefined;
    const update = () => setOnline(networkIsOnline());
    update();
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    return () => {
      window.removeEventListener("online", update);
      window.removeEventListener("offline", update);
    };
  }, []);
  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    const cached = cache.get(key);
    if (cached && !attempt) {
      setState({ key, data: cached, loading: false, error: null });
      return;
    }
    if (!online) {
      setState({
        key,
        data: null,
        loading: false,
        error: new ApiError("Нет подключения к интернету.", 0, {
          code: "network_unavailable",
        }),
      });
      return;
    }
    setState({ key, data: null, loading: true, error: null });
    Promise.resolve()
      .then(() => loader.current(controller.signal))
      .then((data) => {
        if (controller.signal.aborted) return;
        if (cache.size > 100) cache.clear();
        cache.set(key, data);
        setState({ key, data, loading: false, error: null });
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setState({ key, data: null, loading: false, error });
      });
    return () => controller.abort();
  }, [key, enabled, attempt, online]);
  return {
    ...(enabled && state.key === key
      ? state
      : { data: null, loading: enabled, error: null }),
    online,
    retry: () => setAttempt((n) => n + 1),
  };
}
