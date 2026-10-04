import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useApp } from "../state/context.js";
import Icon from "../components/Icon.jsx";
import { Cover } from "../components/Primitives.jsx";
import { TrackList } from "../components/Tracks.jsx";
import { entityKey } from "../lib/library.js";

function trackCount(count) {
  const form = new Intl.PluralRules("ru").select(count);
  return `${count} ${{ one: "трек", few: "трека", many: "треков", other: "трека" }[form]}`;
}

function PlaylistTile({ playlist, own = false }) {
  const app = useApp();
  const [menu, setMenu] = useState(false);
  const root = useRef(null);
  const pinned = own
    ? playlist.pinned === true
    : (app.library.pins || []).includes(entityKey(playlist));
  const liked = own
    ? playlist.liked === true
    : app.library.savedPlaylists.some(
        (item) => entityKey(item) === entityKey(playlist),
      );
  const tracks = playlist.tracks || [];
  const href = own
    ? `/playlist?id=${playlist.id}`
    : `/${playlist.album ? "album" : "playlist"}?id=${encodeURIComponent(playlist.id)}&source=${encodeURIComponent(playlist.source || "soundcloud")}`;
  useEffect(() => {
    if (!menu) return;
    const close = (event) => {
      if (!root.current?.contains(event.target)) setMenu(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [menu]);
  const play = () => {
    const track = firstPlayable(tracks, app.settings.explicit);
    if (!track) {
      app.toast("В плейлисте пока нет треков.");
      return;
    }
    app.play(track, tracks);
  };
  const like = () => {
    if (own) app.likeOwnPlaylist(playlist.id);
    else app.toggleSaved("savedPlaylists", playlist);
  };
  return (
    <div
      className="CollectionPlaylists_item__YeviY CollectionPlaylists_important__oumcA"
      ref={root}
    >
      <article className="collection-playlist playlist-tile">
        <div className="playlist-tile-cover">
          <Cover
            track={{ artwork: playlist.artwork || tracks[0]?.artwork }}
            large
          />
          <button
            className="tile-pin"
            aria-label={pinned ? "Открепить" : "Закрепить"}
            aria-pressed={pinned}
            onClick={() => app.togglePin(playlist)}
          >
            <Icon name={pinned ? "pin_filled_xs" : "pin_xs"} size={18} />
          </button>
          <button
            className="card-play"
            aria-label={`Слушать ${playlist.name}`}
            onClick={play}
          >
            <Icon name="play_filled_l" size={28} />
          </button>
          <button
            className="tile-more"
            aria-label="Действия с плейлистом"
            aria-expanded={menu}
            onClick={() => setMenu((value) => !value)}
          >
            <Icon name="more_xs" size={18} />
          </button>
          <button
            className="tile-like"
            aria-label={liked ? "Убрать отметку" : "Нравится"}
            aria-pressed={liked}
            onClick={like}
          >
            <Icon name={liked ? "liked_xs" : "like_xs"} size={18} />
          </button>
          {menu && (
            <div className="tile-menu" role="menu">
              <button
                role="menuitem"
                onClick={() => {
                  app.togglePin(playlist);
                  setMenu(false);
                }}
              >
                <Icon name={pinned ? "unpin_xxs" : "pin_xxs"} size={18} />
                {pinned ? "Открепить" : "Закрепить"}
              </button>
              <button
                role="menuitem"
                onClick={() => {
                  like();
                  setMenu(false);
                }}
              >
                <Icon name={liked ? "liked_xs" : "like_xs"} size={18} />
                Нравится
              </button>
              <button
                role="menuitem"
                onClick={() => {
                  play();
                  setMenu(false);
                }}
              >
                <Icon name="play_xxs" size={18} />
                Трейлер
              </button>
            </div>
          )}
        </div>
        <Link to={href}>{playlist.name}</Link>
        <span className="muted">
          {playlist.artist || trackCount(playlist.count ?? tracks.length)}
        </span>
      </article>
    </div>
  );
}

function CollectionHeading({ title, to, playlists = false, id }) {
  return (
    <header
      className={`BlockHeader_root__j3mbg collection-block-header ${playlists ? "CollectionPlaylists_header__EDtBS CollectionPlaylists_important__oumcA" : ""}`}
    >
      <h2 id={id} className="BlockHeader_heading__4iqvS">
        <Link className="BlockHeader_title__5xlx6" to={to}>
          {title}
          <Icon
            className="BlockHeader_titleIcon__GQFEK"
            name="arrowRight_xs"
            size={24}
          />
        </Link>
      </h2>
    </header>
  );
}

// DOM composition follows CollectionPage, LikesAndHistory and CollectionPlaylists
// in the original export. All hashed classes resolve to unchanged source CSS.
function hoursLabel(seconds) {
  const hours = Math.round(seconds / 3600);
  if (hours < 1) {
    const minutes = Math.max(1, Math.round(seconds / 60));
    return `${minutes} мин`;
  }
  const form = new Intl.PluralRules("ru").select(hours);
  return `${hours} ${{ one: "час", few: "часа", many: "часов", other: "часа" }[form]}`;
}

function favoriteArtists(library) {
  const rows = new Map();
  const add = (track, seconds = 0) => {
    if (!track?.artist) return;
    const id = track.artistId || track.artist;
    const key = entityKey({ source: track.source, id });
    const row = rows.get(key) || {
      id,
      source: track.source || "soundcloud",
      name: track.artist,
      artwork: track.artwork,
      seconds: 0,
    };
    row.seconds += seconds;
    if (!row.artwork && track.artwork) row.artwork = track.artwork;
    rows.set(key, row);
  };
  library.artists.forEach((artist) => {
    const key = entityKey(artist);
    rows.set(key, {
      id: artist.id,
      source: artist.source || "soundcloud",
      name: artist.name,
      artwork: artist.artwork,
      seconds: rows.get(key)?.seconds || 0,
    });
  });
  [...library.likes, ...library.history].forEach((track) =>
    add(track, track.duration || 0),
  );
  library.listens.forEach((listen) =>
    add(listen.track || listen, listen.track?.duration || 0),
  );
  return [...rows.values()]
    .sort((a, b) => b.seconds - a.seconds || a.name.localeCompare(b.name))
    .slice(0, 12);
}

function firstPlayable(tracks, explicit) {
  return (tracks || []).find(
    (track) =>
      track?.title &&
      track.access !== "blocked" &&
      (explicit || !track.explicit),
  );
}

export default function CollectionOverview({ onCreate }) {
  const app = useApp();
  const { library } = app;
  const [tab, setTab] = useState("created");
  const [albumTab, setAlbumTab] = useState("albums");
  const artists = favoriteArtists(library);
  const likes = library.likes.slice(0, 8);
  return (
    <div className="CollectionPage_root__CZAxL collection-overview">
      <header className="TextHeader_staticItem__OMNew">
        <div className="CollectionPage_header__z193s">
          <h1>Коллекция</h1>
        </div>
      </header>
      <div className="CollectionPage_content__c3f8z">
        <div className="CollectionPage_landing__B4jW_ collection-landing">
          <section
            className="collection-block"
            aria-labelledby="collection-likes-title"
          >
            <CollectionHeading
              title="Мне нравится"
              to="/mymusic/favorite_tracks"
              id="collection-likes-title"
            />
            {likes.length ? (
              <div className="collection-likes">
                <TrackList tracks={likes} columns compact />
              </div>
            ) : (
              <p className="collection-block-text">
                Ставьте лайки трекам, и они появятся тут.
              </p>
            )}
          </section>

          <section
            className="collection-block"
            aria-labelledby="collection-artists-title"
          >
            <CollectionHeading
              title="Любимые исполнители"
              to="/collection/artists"
              id="collection-artists-title"
            />
            {artists.length ? (
              <div className="collection-carousel">
                {artists.map((artist, index) => (
                  <div
                    className="CollectionPlaylists_item__YeviY"
                    key={entityKey(artist)}
                  >
                    <Link
                      className="collection-playlist collection-artist"
                      to={`/artist?id=${encodeURIComponent(artist.id)}&source=${encodeURIComponent(artist.source || "soundcloud")}`}
                    >
                      <span className="artist-cover">
                        <span className="artist-rank">{index + 1}</span>
                        <Cover track={{ artwork: artist.artwork }} large />
                      </span>
                      <span>{artist.name}</span>
                      <span className="muted">
                        {artist.seconds
                          ? hoursLabel(artist.seconds)
                          : "Исполнитель"}
                      </span>
                    </Link>
                  </div>
                ))}
              </div>
            ) : (
              <p className="collection-block-text">
                Здесь появятся исполнители, которых вы слушаете.
              </p>
            )}
          </section>

          <section
            className="CollectionPlaylists_root_withControls__YV7o_ collection-block"
            aria-labelledby="collection-playlists-title"
          >
            <CollectionHeading
              title="Мои плейлисты"
              to="/collection/playlists"
              id="collection-playlists-title"
              playlists
            />
            <div
              className="TabCarousel_root__8DoRy CollectionPlaylists_tabCarousel__hWuL_ CollectionPlaylists_important__oumcA collection-tabs"
              role="tablist"
              aria-label="Плейлисты"
            >
              {[
                ["created", "Вы собрали"],
                ["liked", "Вам понравилось"],
              ].map(([value, label]) => (
                <button
                  key={value}
                  role="tab"
                  id={`collection-tab-${value}`}
                  aria-selected={tab === value}
                  aria-controls={`collection-panel-${value}`}
                  tabIndex={tab === value ? 0 : -1}
                  className="Tab_root__LUukY Tab_tab_size_m__c7tVg CollectionPlaylists_tab__PppbA CollectionPlaylists_important__oumcA"
                  onClick={() => setTab(value)}
                  onKeyDown={(event) => {
                    if (
                      ["ArrowLeft", "ArrowRight", "Home", "End"].includes(
                        event.key,
                      )
                    ) {
                      event.preventDefault();
                      const next =
                        event.key === "Home"
                          ? "created"
                          : event.key === "End"
                            ? "liked"
                            : tab === "created"
                              ? "liked"
                              : "created";
                      setTab(next);
                      document
                        .getElementById(`collection-tab-${next}`)
                        ?.focus();
                    }
                  }}
                >
                  {label}
                </button>
              ))}
            </div>
            <div
              className="CollectionPlaylists_tabPanel__wSwRR"
              role="tabpanel"
              id={`collection-panel-${tab}`}
              aria-labelledby={`collection-tab-${tab}`}
              tabIndex={0}
            >
              {tab === "created" ? (
                <div className="collection-carousel">
                  <div className="CollectionPlaylists_item__YeviY CollectionPlaylists_important__oumcA">
                    <div className="CreatePlaylistCard_root__pMDua CollectionPlaylists_createPlaylistCard__1cMca">
                      <button
                        className="CreatePlaylistCard_button__ZaAtb"
                        onClick={onCreate}
                        aria-label="Создать плейлист"
                      >
                        <Icon
                          className="CreatePlaylistCard_icon__09K9N"
                          name="add_l"
                        />
                      </button>
                      <div className="CreatePlaylistCard_text__dd9Q6">
                        Создать плейлист
                      </div>
                    </div>
                  </div>
                  {library.playlists.map((playlist) => (
                    <PlaylistTile
                      key={`mixora:${playlist.id}`}
                      playlist={playlist}
                      own
                    />
                  ))}
                </div>
              ) : library.savedPlaylists.length ? (
                <div className="collection-carousel">
                  {library.savedPlaylists.map((playlist) => (
                    <PlaylistTile
                      key={entityKey(playlist)}
                      playlist={playlist}
                    />
                  ))}
                </div>
              ) : (
                <div className="collection-liked-empty">
                  <div className="CollectionPlaylistsEmpty_root__KGNv_">
                    Здесь появятся плейлисты, которые вам нравятся.
                  </div>
                </div>
              )}
            </div>
          </section>

          <section
            className={`collection-block ${library.albums.length ? "" : "CollectionAlbumsEmpty_root__xtfuI"}`}
          >
            <CollectionHeading
              title="Любимые альбомы"
              to="/collection/albums"
            />
            <div
              className="collection-tabs"
              role="tablist"
              aria-label="Альбомы"
            >
              {[
                ["albums", "Альбомы"],
                ["upcoming", "Будущие релизы"],
              ].map(([value, label]) => (
                <button
                  key={value}
                  role="tab"
                  aria-selected={albumTab === value}
                  className="Tab_root__LUukY Tab_tab_size_m__c7tVg"
                  onClick={() => setAlbumTab(value)}
                >
                  {label}
                </button>
              ))}
            </div>
            {albumTab === "upcoming" ? (
              <p className="collection-block-text">
                Будущие релизы исполнителей Mixora пока не приходят.
              </p>
            ) : null}
            {albumTab === "albums" &&
              (library.albums.length ? (
                <div className="collection-carousel">
                  {library.albums.map((album) => (
                    <div
                      className="CollectionPlaylists_item__YeviY CollectionPlaylists_important__oumcA"
                      key={entityKey(album)}
                    >
                      <Link
                        className="collection-playlist"
                        to={`/album?id=${encodeURIComponent(album.id)}&source=${encodeURIComponent(album.source || "soundcloud")}`}
                      >
                        <Cover
                          track={{
                            artwork:
                              album.artwork || album.tracks?.[0]?.artwork,
                          }}
                          large
                        />
                        <span>{album.name}</span>
                        <span className="muted">
                          {album.artist || "Альбом"}
                        </span>
                      </Link>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="CollectionAlbumsEmpty_text__fRpx_ collection-block-text">
                  Ставьте лайки альбомам, и они появятся тут.
                </p>
              ))}
          </section>
        </div>
      </div>
    </div>
  );
}
