function distance(left, right) {
  if (Math.abs(left.length - right.length) > 2) return 3;
  const row = Array.from({ length: right.length + 1 }, (_, index) => index);
  for (let i = 1; i <= left.length; i++) {
    let previous = row[0];
    row[0] = i;
    for (let j = 1; j <= right.length; j++) {
      const next = row[j];
      row[j] =
        left[i - 1] === right[j - 1]
          ? previous
          : Math.min(previous, row[j], row[j - 1]) + 1;
      previous = next;
    }
  }
  return row[right.length];
}

export function correctQuery(input, samples = []) {
  const query = String(input || "").trim();
  const folded = query.toLocaleLowerCase();
  if (folded.length < 3) return query;
  let best = "";
  let bestDistance = 3;
  const words = samples.flatMap((sample) =>
    String(sample || "")
      .split(/[^\p{L}\p{N}]+/u)
      .filter((word) => word.length >= 3),
  );
  for (const value of words) {
    const candidate = value.toLocaleLowerCase();
    if (!candidate || candidate === folded) continue;
    const gap = distance(folded, candidate);
    const allowed = Math.max(1, Math.floor(folded.length / 4));
    if (gap > 0 && gap <= allowed && gap < bestDistance) {
      best = value;
      bestDistance = gap;
    }
  }
  return best || query;
}

export function tasteQuery(library = {}) {
  const names = [];
  for (const item of library.searches || []) {
    const text = typeof item === "string" ? item : item?.query || item?.title;
    if (text) names.push(String(text));
  }
  for (const track of [
    ...(library.history || []),
    ...(library.likes || []),
  ].slice(0, 12)) {
    if (track?.artist) names.push(track.artist);
  }
  const counts = new Map();
  for (const name of names) {
    const key = name.trim();
    if (key.length < 2) continue;
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  return [...counts.entries()].sort((a, b) => b[1] - a[1])[0]?.[0] || "";
}
