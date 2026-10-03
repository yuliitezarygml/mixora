import { uniqueTracks } from "./library.js";

const tracks = (value) => (Array.isArray(value) ? value : []);

export function buildWaveRequest({
  library = {},
  catalog = [],
  preferences = {},
  context,
  round = 0,
  explicit = true,
  exclude = [],
} = {}) {
  const likes = tracks(library.likes);
  const history = tracks(library.history);
  const dislikes = tracks(library.dislikes);
  const seeds = uniqueTracks([...likes, ...history, ...tracks(catalog)]).slice(
    0,
    40,
  );

  return {
    preferences,
    context: context || {},
    round,
    explicit,
    exclude: tracks(exclude),
    likes: likes.slice(0, 40),
    history: history.slice(0, 40),
    dislikes: dislikes.slice(0, 80),
    seeds,
  };
}

export function decodeWaveResponse(data) {
  const result = data && typeof data === "object" ? data : {};
  const responseTracks = tracks(result.tracks);
  return {
    tracks: responseTracks,
    sessionId:
      responseTracks.length && typeof result.session_id === "string"
        ? result.session_id
        : "",
    modelVersion: result.model_version || "rules-v0",
  };
}
