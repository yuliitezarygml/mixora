import { useEffect, useRef, useState } from "react";
const cache = new Map();
export function useRemote(key, load, enabled = true) {
  const loader = useRef(load);
  loader.current = load;
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState({
    key: "",
    data: null,
    loading: false,
    error: "",
  });
  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    const cached = cache.get(key);
    if (cached && !attempt) {
      setState({ key, data: cached, loading: false, error: "" });
      return;
    }
    setState({ key, data: null, loading: true, error: "" });
    Promise.resolve()
      .then(() => loader.current(controller.signal))
      .then((data) => {
        if (controller.signal.aborted) return;
        if (cache.size > 100) cache.clear();
        cache.set(key, data);
        setState({ key, data, loading: false, error: "" });
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setState({ key, data: null, loading: false, error: error.message });
      });
    return () => controller.abort();
  }, [key, enabled, attempt]);
  return {
    ...(enabled && state.key === key
      ? state
      : { data: null, loading: enabled, error: "" }),
    retry: () => setAttempt((n) => n + 1),
  };
}
