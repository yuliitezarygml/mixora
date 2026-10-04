import { useState } from "react";
import { Link } from "react-router-dom";
import { Cover, Empty } from "./Primitives.jsx";
import { useApp } from "../state/context.js";
import { providerArtistTracks, providerPlaylistTracks } from "../lib/api.js";
import { entityKey } from "../lib/library.js";
import Icon from "./Icon.jsx";

function firstPlayable(tracks, explicit) {
  return tracks.find(
    (track) => track.access !== "blocked" && (explicit || !track.explicit),
  );
}

export function ArtistCards({ items }) {
  const app = useApp();
  const [busy, setBusy] = useState("");
  const playArtist = async (artist) => {
    const key = entityKey(artist);
    setBusy(key);
    try {
      const tracks = await providerArtistTracks(artist);
      app.play(firstPlayable(tracks, app.settings.explicit), tracks);
    } catch (error) {
      app.toast(error.message);
    } finally {
      setBusy("");
    }
  };
  return (
    <div className="card-grid">
      {items.map((artist) => (
        <article className="artist-card" key={entityKey(artist)}>
          <div className="card-image">
            <Cover track={{ artwork: artist.artwork }} large />
            <button
              className="card-play"
              aria-label={`Слушать ${artist.name}`}
              disabled={busy === entityKey(artist)}
              onClick={() => playArtist(artist)}
            >
              <Icon name="play_filled_l" size={48} />
            </button>
          </div>
          <Link
            to={`/artist?id=${encodeURIComponent(artist.id)}&source=${encodeURIComponent(artist.source || "soundcloud")}`}
          >
            <strong>{artist.name}</strong>
          </Link>
          <span className="muted">Исполнитель</span>
        </article>
      ))}
    </div>
  );
}
export function PlaylistGrid({ items }) {
  const app = useApp();
  const [busy, setBusy] = useState("");
  const playItem = async (item) => {
    const key = entityKey(item);
    setBusy(key);
    try {
      let tracks = item.tracks || [];
      if (!tracks.length) {
        tracks = await providerPlaylistTracks(item);
      }
      app.play(firstPlayable(tracks, app.settings.explicit), tracks);
    } catch (error) {
      app.toast(error.message);
    } finally {
      setBusy("");
    }
  };
  return (
    <div className="card-grid">
      {items.map((item) => (
        <article className="music-card" key={entityKey(item)}>
          <div className="card-image">
            <Cover
              track={{ artwork: item.artwork || item.tracks?.[0]?.artwork }}
              large
            />
            <button
              className="card-play"
              aria-label={`Слушать ${item.name}`}
              disabled={busy === entityKey(item)}
              onClick={() => playItem(item)}
            >
              <Icon name="play_filled_l" size={48} />
            </button>
          </div>
          <Link
            className="card-title"
            to={`/${item.album ? "album" : "playlist"}?id=${encodeURIComponent(item.id)}&source=${encodeURIComponent(item.source || "soundcloud")}`}
          >
            {item.name}
          </Link>
          <span className="muted">
            {item.artist || `${item.count ?? item.tracks?.length ?? 0} треков`}
          </span>
        </article>
      ))}
    </div>
  );
}
export function LoadState({
  remote,
  children,
  empty = false,
  emptyTitle = "Здесь пока нет музыки",
}) {
  const app = useApp();
  if (remote.loading)
    return (
      <div
        className="catalog-skeleton"
        role="status"
        aria-label="Загрузка музыки"
      >
        {[0, 1, 2, 3].map((n) => (
          <div key={n} />
        ))}
        <span className="sr-only">Загрузка музыки…</span>
      </div>
    );
  if (remote.error)
    return (
      <Empty
        title="Не удалось загрузить музыку"
        text={remote.error}
        action={
          <button className="secondary" onClick={remote.retry}>
            Повторить
          </button>
        }
      />
    );
  if (empty)
    return (
      <Empty
        title={emptyTitle}
        action={
          !app.user ? (
            <button className="primary" onClick={() => app.setAuthOpen(true)}>
              Войти и открыть каталог
            </button>
          ) : (
            <Link className="secondary" to="/search">
              Найти музыку
            </Link>
          )
        }
      />
    );
  return children;
}
