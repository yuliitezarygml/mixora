import { useEffect, useRef, useState } from "react";
import {
  loadRecommendedPlaylists,
  recommendationFingerprint,
} from "../lib/recommendedPlaylists.js";

export function useRecommendedPlaylists(options) {
  const fingerprint = recommendationFingerprint(options);
  const [revision, setRevision] = useState(0);
  const [state, setState] = useState({
    key: "",
    playlists: [],
    loading: false,
    failures: 0,
  });
  const latest = useRef(options);
  latest.current = options;
  const key = `${fingerprint}:${revision}`;

  useEffect(() => {
    if (!latest.current.userId) return undefined;
    let active = true;
    let deadline = 0;
    const controller = new AbortController();
    // Coalesce preference/history hydration and rapid actions. Track position
    // is not part of the fingerprint, so normal playback does not refetch.
    const delay = window.setTimeout(async () => {
      deadline = window.setTimeout(() => controller.abort(), 25000);
      try {
        const result = await loadRecommendedPlaylists({
          ...latest.current,
          round: revision,
          signal: controller.signal,
        });
        if (active) setState({ key, ...result, loading: false });
      } catch {
        if (active)
          setState({ key, playlists: [], loading: false, failures: 3 });
      } finally {
        window.clearTimeout(deadline);
      }
    }, 600);
    return () => {
      active = false;
      window.clearTimeout(delay);
      window.clearTimeout(deadline);
      controller.abort();
    };
  }, [key]);

  return {
    ...(options.userId && state.key === key
      ? state
      : { playlists: [], loading: Boolean(options.userId), failures: 0 }),
    refresh: () => setRevision((value) => value + 1),
  };
}
