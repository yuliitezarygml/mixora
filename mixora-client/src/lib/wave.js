import { trackKey, uniqueTracks } from "./library.js";
import { tasteQuery } from "./suggest.js";
export const defaultWave = {
  activity: "any",
  diversity: "any",
  mood: "any",
  language: "any",
};
export const waveActivities = [
  ["wake", "Просыпаюсь"],
  ["road", "В дороге"],
  ["work", "Работаю"],
  ["workout", "Тренируюсь"],
  ["sleep", "Засыпаю"],
];
export const waveCharacters = [
  ["favorite", "Любимое", "liked_m"],
  ["unknown", "Незнакомое", "vibe_xxs"],
  ["popular", "Популярное", "lightning_xxs"],
];
export const waveMoods = [
  ["energetic", "Бодрое", "#ff8a1e"],
  ["happy", "Весёлое", "#7ed321"],
  ["calm", "Спокойное", "#3aa0ff"],
  ["sad", "Грустное", "#7a5cff"],
];
export const waveLanguages = [
  ["russian", "Русский"],
  ["foreign", "Иностранный"],
  ["instrumental", "Без слов"],
];

export function waveExplanation(modelVersion) {
  return String(modelVersion || "").startsWith("gorse-")
    ? "Подбираем по вашей истории прослушиваний и реакциям"
    : "Подбираем по настроению и настройкам волны";
}
const moodQueries = {
  calm: "ambient chill",
  energetic: "electronic dance",
  happy: "indie pop",
  sad: "melancholic acoustic",
  any: "indie electronic",
};
const activityQueries = {
  wake: "morning music",
  road: "driving music",
  work: "focus music",
  workout: "workout music",
  sleep: "sleep music",
};
export function normalizeWave(preferences = {}) {
  const diversity = {
    familiar: "favorite",
    discover: "unknown",
    balanced: "any",
  }[preferences.diversity];
  return {
    ...defaultWave,
    ...preferences,
    activity: activityQueries[preferences.activity]
      ? preferences.activity
      : "any",
    diversity:
      diversity ||
      (["favorite", "unknown", "popular", "any"].includes(preferences.diversity)
        ? preferences.diversity
        : "any"),
    mood: moodQueries[preferences.mood] ? preferences.mood : "any",
    language: ["any", "russian", "foreign", "instrumental"].includes(
      preferences.language,
    )
      ? preferences.language
      : "any",
  };
}
export function waveQuery(preferences, library, context, round = 0) {
  const wave = normalizeWave(preferences);
  if (context?.artist && wave.activity === "any" && wave.mood === "any")
    return context.artist;
  if (context?.genre && wave.activity === "any" && wave.mood === "any")
    return context.genre;
  const parts = [];
  if (activityQueries[wave.activity])
    parts.push(activityQueries[wave.activity]);
  if (wave.mood !== "any") parts.push(moodQueries[wave.mood]);
  if (wave.language === "instrumental") parts.push("instrumental");
  if (!parts.length) {
    const favorite = (library.likes || [])[
      round % Math.max(1, library.likes?.length || 0)
    ];
    if (favorite && wave.diversity !== "unknown") return favorite.artist;
    const taste = tasteQuery(library);
    parts.push(taste && wave.diversity !== "unknown" ? taste : moodQueries.any);
  }
  const query = parts.join(" ");
  return wave.language === "russian" ? `русская музыка ${query}` : query;
}
const instrumental = /instrumental|ambient|piano|classical|оркестр|без слов/i;
export function buildWave(
  candidates,
  library = {},
  preferences = defaultWave,
  { explicit = true, exclude = [], limit = 30, random = Math.random } = {},
) {
  preferences = normalizeWave(preferences);
  const denied = new Set(
    [...(library.dislikes || []), ...exclude].map(trackKey),
  );
  const familiar = new Set(
    [...(library.likes || []), ...(library.history || [])].map(trackKey),
  );
  const favoriteArtists = new Set(
    (library.likes || []).map((t) => t.artistId || t.artist),
  );
  const known =
    preferences.diversity === "favorite"
      ? [...(library.likes || []), ...(library.history || [])]
      : [];
  const eligible = uniqueTracks([...known, ...candidates]).filter((t) => {
    if (
      t.access === "blocked" ||
      denied.has(trackKey(t)) ||
      (!explicit && t.explicit)
    )
      return false;
    if (
      preferences.diversity === "favorite" &&
      familiar.size &&
      !familiar.has(trackKey(t))
    )
      return false;
    if (preferences.diversity === "unknown" && familiar.has(trackKey(t)))
      return false;
    const cyrillic = /[а-яё]/i.test(`${t.title} ${t.artist}`);
    return preferences.language === "russian"
      ? cyrillic
      : preferences.language === "foreign"
        ? !cyrillic
        : true;
  });
  const quiet = eligible.filter((track) =>
    instrumental.test(`${track.title} ${track.genre}`),
  );
  const pool =
    preferences.language === "instrumental" && quiet.length ? quiet : eligible;
  const ranked = pool
    .map((track) => ({
      track,
      score:
        random() +
        (favoriteArtists.has(track.artistId || track.artist) ? 0.3 : 0) +
        (preferences.diversity === "popular"
          ? Math.log10((track.playbackCount || 0) + 1)
          : 0),
    }))
    .sort((a, b) => b.score - a.score)
    .map((x) => x.track);
  // Spread artists through the session; never repeat a track within the batch.
  const result = [];
  while (ranked.length && result.length < limit) {
    const different = ranked.findIndex(
      (t) => t.artist !== result.at(-1)?.artist,
    );
    result.push(ranked.splice(different < 0 ? 0 : different, 1)[0]);
  }
  return result;
}
