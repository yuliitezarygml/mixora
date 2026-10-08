import { api, canonicalTrackReference } from "./api.js";
import { trackKey, uniqueTracks } from "./library.js";
import { defaultWave, normalizeWave } from "./wave.js";
import { buildWaveRequest, decodeWaveResponse } from "./waveRequest.js";

export const recommendedPlaylistPresets = [
  {
    id: "daily",
    name: "Для вас",
    description: "Знакомая музыка и новые находки",
    preferences: {},
  },
  {
    id: "discover",
    name: "Открытия",
    description: "Новое с учётом вашего вкуса",
    preferences: { diversity: "unknown" },
  },
  {
    id: "focus",
    name: "Для работы",
    description: "Спокойнее, чтобы сосредоточиться",
    preferences: { activity: "work", mood: "calm" },
  },
];
const rows = (value) => (Array.isArray(value) ? value : []);
const key = (track) => trackKey(canonicalTrackReference(track));

function recentSearchTracks(library, catalog) {
  return uniqueTracks(
    rows(library.searches)
      .slice(0, 10)
      .flatMap((item) => {
        if (item?.track && key(item.track)) return [item.track];
        const query =
          typeof item === "string" ? item : item?.q || item?.query || "";
        const tokens =
          String(query)
            .toLocaleLowerCase()
            .match(/[\p{L}\p{N}]+/gu) || [];
        if (!tokens.length) return [];
        return rows(catalog)
          .filter((track) => {
            const text = `${track.artist} ${track.title}`.toLocaleLowerCase();
            return (
              (!item?.source ||
                item.source === "local" ||
                item.source === track.source) &&
              tokens.every((token) => text.includes(token))
            );
          })
          .slice(0, 3);
      }),
  ).slice(0, 10);
}

export function recommendationFingerprint(options = {}) {
  const library = options.library || {};
  return JSON.stringify({
    owner: options.userId || "",
    taste: options.taste || null,
    likes: rows(library.likes).map(key).slice(0, 40),
    dislikes: rows(library.dislikes).map(key).slice(0, 80),
    history: rows(library.history).map(key).slice(0, 40),
    searches: rows(library.searches)
      .slice(0, 10)
      .map((item) => [
        typeof item === "string" ? item : item?.q || item?.query || "",
        item?.source || "",
        key(item?.track),
      ]),
    searchedTracks: recentSearchTracks(library, options.catalog).map(key),
    seeds: rows(options.catalog).map(key).slice(0, 40),
    language: normalizeWave(options.preferences).language,
    explicit: options.explicit !== false,
  });
}

export function recommendationRequest(id, options = {}) {
  const preset = recommendedPlaylistPresets.find((item) => item.id === id);
  if (!preset) throw new Error("Неизвестная подборка");
  const searched = recentSearchTracks(options.library || {}, options.catalog);
  const anchor =
    searched[0] ||
    rows(options.library?.likes)[0] ||
    rows(options.library?.history)[0];
  return buildWaveRequest({
    ...options,
    // Search results are weak taste seeds, not fabricated likes. Keep provider
    // identity and use the resolved artist, rather than guessing an artist ID.
    catalog: uniqueTracks([...searched, ...rows(options.catalog)]),
    context: anchor?.artist ? { artist: anchor.artist } : {},
    preferences: {
      ...defaultWave,
      language: normalizeWave(options.preferences).language,
      ...preset.preferences,
    },
  });
}

export function filterRecommendedTracks(
  tracks,
  library = {},
  explicit = true,
  discover = false,
) {
  const denied = new Set(rows(library.dislikes).map(key));
  const known = new Set(
    [...rows(library.likes), ...rows(library.history)].map(key),
  );
  const seen = new Set();
  return rows(tracks)
    .flatMap((track) => {
      const reference = canonicalTrackReference(track);
      const identity = trackKey(reference);
      if (
        !identity ||
        !track.title ||
        track.access === "blocked" ||
        (!explicit && track.explicit) ||
        denied.has(identity) ||
        seen.has(identity) ||
        (discover && known.has(identity))
      )
        return [];
      seen.add(identity);
      return [{ ...track, ...reference }];
    })
    .slice(0, 20);
}

// Reuse the authenticated Wave service, including server-owned preferences,
// history, Gorse and content rankings. Never substitute a static "personal" mix
// on failure. Keep each recommendation's impression session for feedback.
export async function loadRecommendedPlaylists(options, request = api) {
  options.signal?.throwIfAborted();
  const results = await Promise.allSettled(
    recommendedPlaylistPresets.map(async (preset) => {
      const data = await request("/wave", {
        method: "POST",
        body: JSON.stringify(recommendationRequest(preset.id, options)),
        signal: options.signal,
      });
      options.signal?.throwIfAborted();
      const result = decodeWaveResponse(data);
      return {
        ...preset,
        ...result,
        owner: options.userId,
        tracks: filterRecommendedTracks(
          result.tracks,
          options.library,
          options.explicit !== false,
          preset.id === "discover",
        ),
      };
    }),
  );
  options.signal?.throwIfAborted();
  const playlists = results
    .filter((result) => result.status === "fulfilled")
    .map((result) => result.value)
    .filter((item) => item.tracks.length);
  const dailyKeys = new Set(
    playlists.find((item) => item.id === "daily")?.tracks.map(key),
  );
  const focus = playlists.find((item) => item.id === "focus");
  if (focus && focus.modelVersion === "rules-v0") {
    const overlap = focus.tracks.filter((track) =>
      dailyKeys.has(key(track)),
    ).length;
    // Do not shuffle an identical rule fallback and claim it is a distinct
    // semantic focus mix. Expose the lack of suitable catalog candidates.
    focus.limitedCatalog = overlap / focus.tracks.length >= 0.85;
  }
  return {
    playlists,
    failures: results.filter((result) => result.status === "rejected").length,
  };
}
