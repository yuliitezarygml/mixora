import { useEffect, useState } from "react";
import { useApp } from "../state/context.js";
import { searchCatalog, searchSpotifyCatalog } from "../lib/api.js";
import { tasteGenres } from "../lib/tasteProfile.js";
import Icon from "./Icon.jsx";

export default function TasteOnboarding() {
  const app = useApp();
  const [artists, setArtists] = useState(app.taste.artists);
  const [genres, setGenres] = useState(app.taste.genres);
  const [query, setQuery] = useState("");
  const [source, setSource] = useState("soundcloud");
  const [results, setResults] = useState([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const suggested = [
    ...new Map(
      app.catalog
        .filter((track) => track.artist)
        .map((track) => [
          track.artist,
          { name: track.artist, artwork: track.artwork },
        ]),
    ).values(),
  ].slice(0, 24);
  useEffect(() => {
    if (query.trim().length < 2) {
      setResults([]);
      setSearching(false);
      setSearchError("");
      return;
    }
    let active = true;
    const controller = new AbortController();
    setResults([]);
    setSearching(true);
    setSearchError("");
    const timer = setTimeout(async () => {
      try {
        const rows =
          source === "spotify"
            ? await searchSpotifyCatalog(
                "artists",
                query.trim(),
                controller.signal,
              )
            : await searchCatalog("users", query.trim(), controller.signal);
        if (active) setResults(rows);
      } catch (error) {
        if (active && error.name !== "AbortError")
          setSearchError(error.message);
      } finally {
        if (active) setSearching(false);
      }
    }, 350);
    return () => {
      active = false;
      clearTimeout(timer);
      controller.abort();
    };
  }, [query, source]);
  const toggle = (value, list, setter) =>
    setter(
      has(value, list)
        ? list.filter(
            (item) => item.toLocaleLowerCase() !== value.toLocaleLowerCase(),
          )
        : [...list, value],
    );
  const has = (value, list) =>
    list.some((item) => item.toLocaleLowerCase() === value.toLocaleLowerCase());
  async function save(skip = false) {
    setBusy(true);
    setError("");
    try {
      await app.saveTaste({ artists, genres, skip });
    } catch (error) {
      setError(error.message || "Не удалось сохранить интересы");
    } finally {
      setBusy(false);
    }
  }
  const displayed = query.trim().length >= 2 ? results : suggested;
  return (
    <div
      className="auth-wall taste-wall"
      role="dialog"
      aria-modal="true"
      aria-label="Настройте вашу музыку"
    >
      <div className="auth-shade" />
      <section className="auth-card taste-card">
        <header className="auth-card-bar">
          <span>Ваш музыкальный вкус</span>
          <button
            className="auth-icon"
            aria-label="Закрыть выбор интересов"
            disabled={busy}
            onClick={() => app.setTasteOpen(false)}
          >
            <Icon name="close_xs" size={20} />
          </button>
        </header>
        <h2>Кого вы любите слушать?</h2>
        <p className="muted">
          Выберите минимум 5 артистов. Моя волна начнёт с ваших интересов, затем
          будет учиться по прослушиваниям и реакциям.
        </p>
        <div className="taste-search">
          <input
            aria-label="Найти артиста"
            placeholder="Имя артиста"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <select
            aria-label="Источник поиска артистов"
            value={source}
            onChange={(e) => setSource(e.target.value)}
          >
            <option value="soundcloud">SoundCloud</option>
            <option value="spotify">Spotify</option>
          </select>
        </div>
        <p className="taste-count" aria-live="polite">
          Выбрано артистов: {artists.length} / минимум 5
        </p>
        {!!artists.length && (
          <div className="taste-chips" aria-label="Выбранные артисты">
            {artists.map((name) => (
              <button
                key={name}
                disabled={busy}
                onClick={() => toggle(name, artists, setArtists)}
              >
                {name} ×
              </button>
            ))}
          </div>
        )}
        {searching && <p role="status">Ищем артистов…</p>}
        {searchError && <p role="alert">{searchError}</p>}
        <div className="taste-artists">
          {displayed.map((artist) => (
            <button
              key={`${artist.source || "catalog"}:${artist.id || artist.name}`}
              className={has(artist.name, artists) ? "selected" : ""}
              aria-pressed={has(artist.name, artists)}
              disabled={
                busy || (!has(artist.name, artists) && artists.length >= 30)
              }
              onClick={() => toggle(artist.name, artists, setArtists)}
            >
              <ArtistImage artwork={artist.artwork} />
              <span>{artist.name}</span>
            </button>
          ))}
        </div>
        {!searching &&
          !searchError &&
          query.trim().length >= 2 &&
          !displayed.length && (
            <p>Ничего не найдено. Попробуйте другой источник.</p>
          )}
        <h3>Какие жанры вам близки?</h3>
        <p className="muted">
          Можно выбрать несколько или оставить без ограничений.
        </p>
        <div className="taste-chips">
          {tasteGenres.map(([id, label]) => (
            <button
              key={id}
              aria-pressed={genres.includes(id)}
              className={genres.includes(id) ? "selected" : ""}
              disabled={busy}
              onClick={() => toggle(id, genres, setGenres)}
            >
              {label}
            </button>
          ))}
        </div>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        <footer className="taste-actions">
          <button
            className="auth-submit"
            disabled={busy || artists.length < 5}
            onClick={() => save()}
          >
            {busy ? "Сохраняем…" : "Настроить Мою волну"}
          </button>
          <button
            className="text-button"
            disabled={busy}
            onClick={() => save(true)}
          >
            Пропустить пока
          </button>
        </footer>
      </section>
    </div>
  );
}

function ArtistImage({ artwork }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [artwork]);
  return artwork && !failed ? (
    <img src={artwork} alt="" onError={() => setFailed(true)} />
  ) : (
    <Icon name="artist_xxs" size={76} />
  );
}
