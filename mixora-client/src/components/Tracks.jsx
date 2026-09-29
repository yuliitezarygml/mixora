import { useState } from "react";
import { Link } from "react-router-dom";
import { useApp } from "../state/context.js";
import { trackKey, duration } from "../lib/library.js";
import Icon from "./Icon.jsx";
import { Cover, IconButton, Modal, Empty } from "./Primitives.jsx";
export function TrackMenu({ track, onClose, playlistId }) {
  const app = useApp();
  const [view, setView] = useState("main"),
    [name, setName] = useState("");
  return (
    <Modal
      title={view === "playlists" ? "Добавить в плейлист" : track.title}
      onClose={onClose}
    >
      {view === "main" ? (
        <div className="action-list">
          <button
            onClick={() => {
              app.startWave({ artist: track.artist, title: track.title });
              onClose();
            }}
          >
            <Icon name="vibe_xxs" />
            Моя волна по треку
          </button>
          <Link
            to={`/album/track?id=${encodeURIComponent(track.id)}`}
            onClick={onClose}
          >
            <Icon name="info_xxs" />О треке
          </Link>
          <button
            onClick={() => {
              app.addQueue(track);
              onClose();
            }}
          >
            <Icon name="playLast_xxs" />
            Добавить в очередь
          </button>
          <button onClick={() => setView("playlists")}>
            <Icon name="addToPlaylist_xxs" />
            Добавить в плейлист
          </button>
          <button
            onClick={() => {
              app.toggleLike(track);
              onClose();
            }}
          >
            <Icon name="like_xs" />
            Нравится / убрать отметку
          </button>
          {playlistId && (
            <button
              onClick={() => {
                app.removeFromPlaylist(playlistId, track);
                onClose();
              }}
            >
              <Icon name="bucket_xxs" />
              Убрать из плейлиста
            </button>
          )}
          <button
            onClick={() => {
              app.dislike(track);
              onClose();
            }}
          >
            <Icon name="dislike_xs" />
            Не нравится
          </button>
        </div>
      ) : (
        <div className="action-list">
          <p className="muted">Плейлисты аккаунта Mixora</p>
          {app.library.playlists.map((p) => (
            <button
              key={p.id}
              onClick={() => {
                app.addToPlaylist(p.id, track);
                onClose();
              }}
            >
              <Icon name="playlist_xs" />
              {p.name}
            </button>
          ))}
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (!name.trim()) return;
              const p = app.createPlaylist(name);
              app.addToPlaylist(p.id, track);
              onClose();
            }}
          >
            <input
              aria-label="Название нового плейлиста"
              placeholder="Новый плейлист"
              value={name}
              maxLength={120}
              onChange={(e) => setName(e.target.value)}
              required
            />
            <button className="primary" type="submit">
              Создать и добавить
            </button>
          </form>
        </div>
      )}
    </Modal>
  );
}
export function TrackList({
  tracks,
  numbered = false,
  playlistId,
  columns = false,
  compact = false,
  emptyText = "Попробуйте другой запрос или выберите музыку в поиске.",
  onActivate,
}) {
  const app = useApp();
  const [menu, setMenu] = useState(null);
  const visible = tracks.filter((t) => app.settings.explicit || !t.explicit);
  if (!visible.length)
    return <Empty title="Треков пока нет" text={emptyText} />;
  return (
    <>
      <div className={`track-list ${columns ? "track-list-columns" : ""}`}>
        {visible.map((t, i) => {
          const active = app.current && trackKey(t) === trackKey(app.current);
          const liked = app.library.likes.some(
            (x) => trackKey(x) === trackKey(t),
          );
          return (
            <div
              className={`track-row CommonTrack_root__i6shE ${active ? "current CommonTrack_root_current__MNrpS" : ""}`}
              key={trackKey(t)}
            >
              <button
                className="track-play"
                aria-label={`${active && app.playing ? "Приостановить" : "Слушать"} ${t.title}`}
                onClick={() => {
                  if (active) app.toggle();
                  else {
                    onActivate?.(t);
                    app.play(t, visible);
                  }
                }}
              >
                {numbered ? (
                  <span className="track-number">{i + 1}</span>
                ) : (
                  <Cover track={t} />
                )}
                <Icon name={active && app.playing ? "pause_xs" : "play_xs"} />
              </button>
              <div className="track-meta">
                <button
                  className="track-title"
                  onClick={() => {
                    onActivate?.(t);
                    app.play(t, visible);
                  }}
                >
                  {t.title}
                  {t.explicit && <span className="explicit">E</span>}
                </button>
                <Link
                  className="artist-link"
                  to={`/artist?id=${encodeURIComponent(t.artistId || t.artist)}`}
                >
                  {t.artist}
                </Link>
              </div>
              {!compact && (
                <span className="source-label">
                  {t.access === "preview"
                    ? "Фрагмент"
                    : t.access === "blocked"
                      ? "Недоступен"
                      : ""}
                </span>
              )}
              <IconButton
                label={
                  liked
                    ? `Убрать из любимого: ${t.title}`
                    : `Нравится: ${t.title}`
                }
                icon={liked ? "liked_xs" : "like_xs"}
                active={liked}
                onClick={() => app.toggleLike(t)}
              />
              <span className="duration">{duration(t.duration)}</span>
              <IconButton
                icon="more_xs"
                label={`Действия: ${t.title}`}
                onClick={() => setMenu(t)}
              />
            </div>
          );
        })}
      </div>
      {menu && (
        <TrackMenu
          track={menu}
          playlistId={playlistId}
          onClose={() => setMenu(null)}
        />
      )}
    </>
  );
}
export function TrackCards({ tracks, onActivate }) {
  const app = useApp();
  return (
    <div className="card-grid">
      {tracks
        .filter((t) => app.settings.explicit || !t.explicit)
        .map((t) => (
          <article className="music-card" key={trackKey(t)}>
            <div className="card-image TrackCard_coverBlock__WdvvQ">
              <Cover track={t} large />
              <button
                className="card-play"
                aria-label={`Слушать ${t.title}`}
                onClick={() => {
                  onActivate?.(t);
                  app.play(t, tracks);
                }}
              >
                <Icon name="play_filled_l" size={48} />
              </button>
            </div>
            <button
              className="card-title TrackCard_title__BVLuv"
              onClick={() => {
                onActivate?.(t);
                app.play(t, tracks);
              }}
            >
              {t.title}
            </button>
            <Link
              className="artist-link"
              to={`/artist?id=${encodeURIComponent(t.artistId || t.artist)}`}
            >
              {t.artist}
            </Link>
            {t.access === "preview" && (
              <span className="card-source">Фрагмент</span>
            )}
          </article>
        ))}
    </div>
  );
}
