import { useEffect, useRef, useState } from "react";
import { api, put } from "../lib/api.js";
import { emptyTaste, normalizeTaste } from "../lib/tasteProfile.js";

export function useTasteProfile(userId, userRef) {
  const revision = useRef(0);
  const [state, setState] = useState({
    owner: "",
    profile: emptyTaste(),
    ready: false,
    open: false,
  });
  useEffect(() => {
    if (!userId) return;
    let active = true;
    const loadRevision = ++revision.current;
    const controller = new AbortController();
    setState({
      owner: userId,
      profile: emptyTaste(),
      ready: false,
      open: false,
    });
    api("/me/taste", { signal: controller.signal })
      .then((result) => {
        if (
          !active ||
          revision.current !== loadRevision ||
          userRef.current?.id !== userId
        )
          return;
        const profile = normalizeTaste(result);
        setState({
          owner: userId,
          profile,
          ready: true,
          open: !profile.completed,
        });
      })
      .catch((error) => {
        if (
          active &&
          revision.current === loadRevision &&
          error.name !== "AbortError"
        )
          setState({
            owner: userId,
            profile: emptyTaste(),
            ready: true,
            open: false,
          });
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [userId]);
  const current =
    state.owner === userId
      ? state
      : { profile: emptyTaste(), ready: !userId, open: false };
  return {
    profile: current.profile,
    ready: current.ready,
    open: current.open,
    setOpen: (open) =>
      setState((previous) => ({ ...previous, owner: userId, open })),
    save: async (input) => {
      if (!userId) throw new Error("Войдите в аккаунт");
      const profile = normalizeTaste(await put("/me/taste", input));
      if (userRef.current?.id !== userId) return false;
      revision.current++;
      setState({ owner: userId, profile, ready: true, open: false });
      return true;
    },
  };
}
