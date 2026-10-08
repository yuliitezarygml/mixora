export const tasteGenres = [
  ["pop", "Поп"],
  ["hip-hop", "Хип-хоп"],
  ["rock", "Рок"],
  ["electronic", "Электроника"],
  ["indie", "Инди"],
  ["jazz", "Джаз"],
  ["classical", "Классика"],
  ["ambient", "Эмбиент"],
  ["metal", "Метал"],
  ["rnb", "R&B"],
  ["folk", "Фолк"],
  ["dance", "Танцевальная"],
];
export const emptyTaste = () => ({ artists: [], genres: [], completed: false });
export function normalizeTaste(value) {
  const artists = [
    ...new Map(
      (Array.isArray(value?.artists) ? value.artists : [])
        .filter((item) => typeof item === "string" && item.trim())
        .map((item) => item.trim())
        .map((item) => [item.toLocaleLowerCase(), item]),
    ).values(),
  ].slice(0, 30);
  return {
    artists,
    genres: [
      ...new Set(
        (Array.isArray(value?.genres) ? value.genres : []).filter((genre) =>
          tasteGenres.some(([id]) => id === genre),
        ),
      ),
    ],
    completed: value?.completed === true,
  };
}
export function tasteContext(taste, round = 0) {
  const value = normalizeTaste(taste);
  const pick = (items) =>
    items[((round % items.length) + items.length) % items.length];
  return {
    ...(value.artists.length ? { artist: pick(value.artists) } : {}),
    ...(value.genres.length ? { genre: pick(value.genres) } : {}),
  };
}
