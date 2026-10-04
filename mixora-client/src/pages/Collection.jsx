import { useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useApp } from "../state/context.js";
import { TrackList } from "../components/Tracks.jsx";
import { PlaylistGrid } from "../components/Catalog.jsx";
import { Tabs, Empty, Modal } from "../components/Primitives.jsx";
import Icon from "../components/Icon.jsx";
import CollectionOverview from "./CollectionOverview.jsx";
import { entityKey, trackKey } from "../lib/library.js";
export function PlaylistCards() {
  const app = useApp();
  return (
    <div className="card-grid">
      {app.library.playlists.map((p) => (
        <Link className="music-card" key={p.id} to={`/playlist?id=${p.id}`}>
          <div className="playlist-cover">
            <Icon name="playlist_xl" size={70} />
          </div>
          <strong>{p.name}</strong>
          <span className="muted">{p.tracks.length} треков</span>
        </Link>
      ))}
    </div>
  );
}
export function CreatePlaylist({ onClose }) {
  const app = useApp(),
    navigate = useNavigate();
  const [name, setName] = useState("");
  return (
    <Modal title="Новый плейлист" onClose={onClose}>
      <form
        className="auth-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (!name.trim()) return;
          const p = app.createPlaylist(name);
          if (!p) return;
          onClose();
          navigate(`/playlist?id=${p.id}`);
        }}
      >
        <label>
          Название
          <input
            autoFocus
            maxLength={120}
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Например, для долгой дороги"
          />
        </label>
        <p className="muted">Плейлист синхронизируется с аккаунтом Mixora.</p>
        <button className="primary" type="submit">
          Создать
        </button>
      </form>
    </Modal>
  );
}
export default function Collection() {
  const app = useApp(),
    { pathname } = useLocation();
  const [create, setCreate] = useState(false);
  if (pathname === "/collection") {
    return (
      <>
        <CollectionOverview onCreate={() => setCreate(true)} />
        {create && <CreatePlaylist onClose={() => setCreate(false)} />}
      </>
    );
  }
  const tabs = [
    ["Всё", "/collection"],
    ["Треки", "/mymusic/favorite_tracks"],
    ["Плейлисты", "/collection/playlists"],
    ["Альбомы", "/collection/albums"],
    ["Исполнители", "/collection/artists"],
    ["Детям", "/collection/kids"],
    ["Клипы", "/collection/clips"],
  ];
  const kids = pathname.includes("/kids");
  const podcasts =
    pathname.includes("non-music") || pathname.includes("/shelf");
  const clips = pathname.includes("clips");
  const isFavorites = pathname === "/mymusic/favorite_tracks";
  const isDislikes = pathname === "/collection/dislikes";
  const isDownloads = pathname.includes("downloads");
  const isPlaylists = pathname.includes("playlists") && !kids;
  const isAlbums = pathname === "/collection/albums";
  const title = isFavorites
    ? "Мне нравится"
    : isDislikes
      ? "Не нравится"
      : isDownloads
        ? "Скачанные треки"
        : kids
          ? "Детям"
          : podcasts
            ? "Подкасты и книги"
            : clips
              ? "Клипы"
              : isPlaylists
                ? "Плейлисты"
                : isAlbums
                  ? "Альбомы"
                  : "Коллекция";
  let body;
  if (isFavorites) body = <TrackList tracks={app.library.likes} />;
  else if (isDislikes)
    body = (
      <>
        <p className="muted">Вы можете вернуть треки в рекомендации.</p>
        {app.library.dislikes.map((t) => (
          <div className="setting-row" key={trackKey(t)}>
            <span>
              {t.title} — {t.artist}
            </span>
            <button onClick={() => app.clearTrackPreference(t)}>Вернуть</button>
          </div>
        ))}
        {!app.library.dislikes.length && (
          <Empty icon="dislike_s" title="Нет исключённых треков" />
        )}
      </>
    );
  else if (kids)
    body = (
      <>
        <div className="chip-row">
          {[
            ["Треки", "/collection/kids/tracks"],
            ["Плейлисты", "/collection/kids/playlists"],
            ["Альбомы", "/collection/kids/albums"],
          ].map(([label, to]) => (
            <Link
              key={to}
              to={to}
              className={pathname === to ? "selected" : ""}
            >
              {label}
            </Link>
          ))}
        </div>
        <Empty
          icon="kids_m"
          title="Детская коллекция пока пуста"
          text="Откройте раздел «Детям» и сохраните понравившиеся треки в «Мне нравится»."
          action={
            <Link className="primary" to="/kids">
              Слушать детское
            </Link>
          }
        />
      </>
    );
  else if (isPlaylists)
    body = (
      <>
        <div className="chip-row">
          <Link
            to="/collection/playlists/created"
            className={!pathname.endsWith("liked") ? "selected" : ""}
          >
            Мои плейлисты
          </Link>
          <Link
            to="/collection/playlists/liked"
            className={pathname.endsWith("liked") ? "selected" : ""}
          >
            Мне нравятся
          </Link>
        </div>
        {pathname.endsWith("liked") ? (
          app.library.savedPlaylists.length ? (
            <PlaylistGrid items={app.library.savedPlaylists} />
          ) : (
            <Empty
              icon="playlist_xl"
              title="Нет сохранённых подборок"
              text="Откройте плейлист SoundCloud и сохраните его в коллекцию."
            />
          )
        ) : app.library.playlists.length ? (
          <PlaylistCards />
        ) : (
          <Empty
            icon="playlist_xl"
            title="Соберите свою музыку"
            text="Создайте первый плейлист и добавьте в него любимые треки."
            action={
              <button className="primary" onClick={() => setCreate(true)}>
                Создать плейлист
              </button>
            }
          />
        )}
      </>
    );
  else if (isAlbums)
    body = app.library.albums.length ? (
      <PlaylistGrid items={app.library.albums} />
    ) : (
      <Empty
        icon="album_xl"
        title="Нет сохранённых альбомов"
        text="Откройте альбом и нажмите «Сохранить в коллекцию»."
      />
    );
  else if (pathname.includes("/artists"))
    body = app.library.artists.length ? (
      <div className="card-grid">
        {app.library.artists.map((a) => (
          <Link
            className="artist-card"
            to={`/artist?id=${encodeURIComponent(a.id)}&source=${encodeURIComponent(a.source || "soundcloud")}`}
            key={entityKey(a)}
          >
            {a.artwork ? (
              <img src={a.artwork} alt="" />
            ) : (
              <Icon name="artist_xxs" size={80} />
            )}
            <strong>{a.name}</strong>
          </Link>
        ))}
      </div>
    ) : (
      <Empty
        icon="artist_xxs"
        title="Ваши любимые исполнители"
        text="Откройте страницу исполнителя и нажмите «Подписаться»."
      />
    );
  else {
    body = (
      <>
        {podcasts && (
          <div className="chip-row">
            {[
              ["Все", "/collection/shelf"],
              ["Избранное", "/collection/shelf/liked"],
              ["Новые выпуски", "/collection/shelf/new-episodes"],
              ["Недавно слушали", "/collection/shelf/recently-played"],
            ].map(([label, to]) => (
              <Link
                key={to}
                to={to}
                className={pathname === to ? "selected" : ""}
              >
                {label}
              </Link>
            ))}
          </div>
        )}
        <Empty
          icon={isDownloads ? "download_l" : clips ? "clip_xl" : "non_music_m"}
          title={
            isDownloads
              ? "Скачанные треки"
              : clips
                ? "Клипы"
                : "Подкасты и книги"
          }
          text={
            isDownloads
              ? "Офлайн-загрузка ещё не подключена. Музыку можно слушать онлайн."
              : clips
                ? "Источник видео ещё не подключён."
                : "В коллекции пока нет сохранённых выпусков. SoundCloud не отдаёт отдельную полку подкастов."
          }
          action={
            <Link className="primary" to={podcasts ? "/non-music" : "/search"}>
              {podcasts ? "Открыть подкасты" : "Найти музыку"}
            </Link>
          }
        />
      </>
    );
  }
  return (
    <div className="page-padding">
      <div className="page-heading">
        <div>
          <h1>{title}</h1>
          <p className="muted">
            На этом устройстве{app.user ? ` · ${app.user.display_name}` : ""}
          </p>
        </div>
        <button
          className="round-button"
          aria-label="Создать плейлист"
          onClick={() => setCreate(true)}
        >
          <Icon name="add_l" />
        </button>
      </div>
      <Tabs
        items={tabs.map(([label, to]) => ({
          label,
          to,
          active:
            to === pathname ||
            (to !== "/collection" && pathname.startsWith(to)),
        }))}
      />
      {body}
      {create && <CreatePlaylist onClose={() => setCreate(false)} />}
    </div>
  );
}
