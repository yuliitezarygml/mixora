import { useState } from "react";
import {
  Link,
  useLocation,
  useNavigate,
  useSearchParams,
} from "react-router-dom";
import { useApp } from "../state/context.js";
import {
  api,
  catalogResource,
  collectionItems,
  soundcloudArtist,
  soundcloudPlaylist,
  soundcloudTrack,
  sourceLabel,
  spotifyArtist,
  spotifyPlaylist,
  spotifyTrack,
} from "../lib/api.js";
import { useRemote } from "../lib/useRemote.js";
import {
  duration,
  findKnownTrack,
  uniqueTracks,
  shuffleTracks,
} from "../lib/library.js";
import { TrackList } from "../components/Tracks.jsx";
import {
  ArtistCards,
  PlaylistGrid,
  LoadState,
} from "../components/Catalog.jsx";
import {
  Section,
  Tabs,
  Cover,
  IconButton,
  Empty,
  Modal,
} from "../components/Primitives.jsx";
import Icon from "../components/Icon.jsx";
const artistTabs = [
  ["Главное", ""],
  ["Треки", "/tracks"],
  ["Альбомы", "/albums"],
  ["Дискография", "/discography"],
  ["Сборники", "/compilations"],
  ["Похожие", "/similar"],
  ["Клипы", "/videos"],
  ["Знакомое", "/familiar"],
];
export default function Details() {
  const app = useApp(),
    { pathname } = useLocation(),
    [params] = useSearchParams(),
    navigate = useNavigate();
  const [modal, setModal] = useState(null),
    [filter, setFilter] = useState(""),
    [sort, setSort] = useState("default"),
    [draft, setDraft] = useState({ name: "", description: "" });
  const artist = pathname.startsWith("/artist"),
    label = pathname.startsWith("/label"),
    trackPage = pathname === "/album/track";
  const isPlaylist = pathname.includes("playlist"),
    isAlbum = !artist && !label && !trackPage && !isPlaylist;
  const id = params.get("id") || (artist ? app.catalog[0]?.artistId : "");
  const source = params.get("source") || "soundcloud";
  const personal = app.library.playlists.find((p) => p.id === id);
  const cachedTrack = findKnownTrack(app.catalog, app.library, source, id);
  const remote = useRemote(
    `${app.user?.id}:entity:${source}:${artist || label ? "users" : trackPage ? "tracks" : "playlists"}:${id}`,
    async (signal) => {
      if (source === "spotify") {
        if (artist || label) {
          const profile = await api(
            `/spotify/artists/${encodeURIComponent(id)}`,
            { signal },
          );
          return {
            ...spotifyArtist(profile),
            description: (profile?.genres || []).join(", "),
            tracks: (profile?.top_tracks || []).map(spotifyTrack),
            playlists: [],
          };
        }
        if (trackPage) {
          return spotifyTrack(
            await api(`/spotify/tracks/${encodeURIComponent(id)}`, { signal }),
          );
        }
        const resource = isAlbum ? "albums" : "playlists";
        return spotifyPlaylist(
          await api(`/spotify/${resource}/${encodeURIComponent(id)}`, {
            signal,
          }),
          { album: isAlbum },
        );
      }
      if (artist || label) {
        const [profile, tracks, playlists] = await Promise.all([
          catalogResource("users", id, "", signal),
          catalogResource("users", id, "tracks", signal),
          catalogResource("users", id, "playlists", signal),
        ]);
        return {
          ...soundcloudArtist(profile),
          tracks: collectionItems(tracks).map(soundcloudTrack),
          playlists: collectionItems(playlists).map(soundcloudPlaylist),
        };
      }
      if (trackPage)
        return soundcloudTrack(await catalogResource("tracks", id, "", signal));
      return soundcloudPlaylist(
        await catalogResource("playlists", id, "", signal),
      );
    },
    !!app.user &&
      !!id &&
      !personal &&
      ((source === "soundcloud" && cachedTrack?.source !== "local") ||
        source === "spotify"),
  );
  const localArtistTracks = app.catalog.filter(
    (track) =>
      track.source === source &&
      (track.artistId === id || (!track.artistId && track.artist === id)),
  );
  const localEntity =
    artist || label
      ? {
          id,
          name: localArtistTracks[0]?.artist,
          artwork: localArtistTracks[0]?.artwork,
          tracks: localArtistTracks,
          playlists: [],
        }
      : trackPage
        ? cachedTrack
        : null;
  const entity = personal || remote.data || localEntity || {};
  const tracks = trackPage
    ? entity.title
      ? [entity]
      : []
    : entity.tracks || [];
  const title =
    entity.name ||
    entity.title ||
    (artist
      ? "Исполнитель"
      : label
        ? "Лейбл"
        : trackPage
          ? "Трек"
          : isPlaylist
            ? "Плейлист"
            : "Альбом");
  const sub = pathname.split("/")[2] || "";
  const known = uniqueTracks([
    ...app.library.likes,
    ...app.library.history,
  ]).filter((track) => track.source === source && track.artistId === id);
  const allArtists = [
    ...new Map(
      app.catalog
        .filter((t) => t.artistId !== id)
        .map((t) => [
          `${t.source}:${t.artistId || t.artist}`,
          {
            id: t.artistId || t.artist,
            source: t.source,
            name: t.artist,
            artwork: t.artwork,
          },
        ]),
    ).values(),
  ];
  const savedField = isAlbum ? "albums" : "savedPlaylists";
  const saved = app.library[savedField].some(
    (x) => x.id === id && (x.source || "soundcloud") === source,
  );
  const followed = app.library.artists.some(
    (x) => x.id === id && (x.source || "soundcloud") === source,
  );
  let displayTracks = (sub === "familiar" ? known : tracks).filter((t) =>
    (t.title + " " + t.artist).toLowerCase().includes(filter.toLowerCase()),
  );
  if (sort === "title")
    displayTracks = [...displayTracks].sort((a, b) =>
      a.title.localeCompare(b.title),
    );
  if (sort === "duration")
    displayTracks = [...displayTracks].sort((a, b) => a.duration - b.duration);
  const route = label ? "/label" : "/artist";
  const tabs = label
    ? [
        ["Главное", ""],
        ["Исполнители", "/artists"],
        ["Альбомы", "/albums"],
      ]
    : artistTabs;
  return (
    <div
      className={`detail-page ${artist ? "ArtistPage_root__QPg3p" : isPlaylist ? "PlaylistPage_root__ajyaP" : ""}`}
    >
      <section
        className={`detail-hero PageHeaderBase_root__xMIBu PageHeaderBase_root_withCover__JIKxy ${artist ? "artist-hero" : ""}`}
      >
        <div
          className={`detail-cover PageHeaderBase_coverCell__nBx4c ${artist ? "round" : ""}`}
        >
          <Cover
            track={{ artwork: entity.artwork || tracks[0]?.artwork }}
            large
          />
        </div>
        <div className="detail-info PageHeaderBase_content___DNyv">
          <span className="eyebrow">
            {artist
              ? "Исполнитель"
              : label
                ? "Лейбл"
                : trackPage
                  ? "Трек"
                  : isPlaylist
                    ? "Плейлист"
                    : "Альбом"}
          </span>
          <h1 className="PageHeaderTitle_heading__UADXi PageHeaderTitle_font_short__76VRG">
            {title}
          </h1>
          <p className="muted">
            {entity.artist || app.user?.display_name || sourceLabel(source)}
            {tracks.length
              ? ` · ${tracks.length} треков · ${duration(tracks.reduce((sum, t) => sum + (t.duration || 0), 0))}`
              : ""}
          </p>
          <div className="detail-actions PageHeaderBase_controls__HzGgE">
            <button
              className="primary"
              disabled={
                !tracks.some(
                  (track) =>
                    track.access !== "blocked" &&
                    (app.settings.explicit || !track.explicit),
                )
              }
              onClick={() =>
                app.play(
                  tracks.find(
                    (track) =>
                      track.access !== "blocked" &&
                      (app.settings.explicit || !track.explicit),
                  ),
                  tracks,
                )
              }
            >
              <Icon name="play_xs" />
              Слушать
            </button>
            <button
              className="secondary"
              disabled={!tracks.length}
              onClick={() =>
                app.startWave({ artist: tracks[0]?.artist || title, title })
              }
            >
              <Icon name="vibe_xxs" />
              Моя волна
            </button>
            {artist || label ? (
              <button
                className="secondary"
                disabled={!entity.name}
                onClick={() =>
                  app.toggleSaved("artists", {
                    id,
                    source,
                    name: title,
                    artwork: entity.artwork,
                  })
                }
              >
                {followed ? "Вы подписаны" : "Подписаться"}
              </button>
            ) : trackPage ? (
              <IconButton
                icon="like_xs"
                label="Нравится"
                onClick={() => tracks[0] && app.toggleLike(tracks[0])}
              />
            ) : !personal ? (
              <IconButton
                icon={saved ? "liked_xs" : "like_xs"}
                label={saved ? "Убрать из коллекции" : "Сохранить в коллекцию"}
                onClick={() => app.toggleSaved(savedField, entity)}
                disabled={!entity.id}
              />
            ) : null}
            <IconButton
              icon="shuffle_xs"
              label="Перемешать"
              disabled={!tracks.length}
              onClick={() => {
                const list = shuffleTracks(tracks, -1);
                app.play(list[0], list);
              }}
            />
            <IconButton
              icon="info_xxs"
              label="Описание"
              onClick={() => setModal("about")}
            />
            {personal && (
              <>
                <button
                  className="text-button"
                  onClick={() => {
                    setDraft({
                      name: personal.name,
                      description: personal.description || "",
                    });
                    setModal("edit");
                  }}
                >
                  Редактировать
                </button>
                <IconButton
                  icon="bucket_xxs"
                  label="Удалить плейлист"
                  onClick={() => setModal("delete")}
                />
              </>
            )}
          </div>
        </div>
      </section>
      <div className="page-padding">
        {(artist || label) && (
          <Tabs
            items={tabs.map(([text, suffix]) => ({
              label: text,
              to: `${route}${suffix}?id=${encodeURIComponent(id || "")}&source=${encodeURIComponent(source)}`,
              active: pathname === route + suffix,
            }))}
          />
        )}
        <LoadState
          remote={personal ? { loading: false } : remote}
          source={source}
        >
          {!id || (!entity.name && !entity.title && !tracks.length) ? (
            <Empty
              title="Музыка не найдена"
              text={
                app.user
                  ? "Выберите исполнителя, трек или подборку в поиске."
                  : "Войдите, чтобы открыть данные SoundCloud."
              }
              action={
                <Link className="primary" to="/search">
                  Поиск
                </Link>
              }
            />
          ) : ["albums", "discography", "compilations"].includes(sub) ? (
            <Section title={sub === "compilations" ? "Сборники" : "Альбомы"}>
              <PlaylistGrid
                items={(entity.playlists || []).filter((p) =>
                  sub === "compilations"
                    ? !p.album
                    : sub === "discography" || p.album,
                )}
              />
              {!(entity.playlists || []).length && (
                <Empty
                  icon="album_xl"
                  title="У исполнителя нет опубликованных подборок"
                />
              )}
            </Section>
          ) : sub === "similar" ? (
            <Section title="Другие исполнители из вашей музыки">
              <ArtistCards items={allArtists} />
            </Section>
          ) : sub === "artists" ? (
            <ArtistCards
              items={[
                ...new Map(
                  tracks.map((t) => [
                    `${t.source}:${t.artistId || t.artist}`,
                    {
                      id: t.artistId || t.artist,
                      source: t.source,
                      name: t.artist,
                      artwork: t.artwork,
                    },
                  ]),
                ).values(),
              ]}
            />
          ) : ["videos", "concerts"].includes(sub) ? (
            <Empty
              icon={sub === "videos" ? "clip_xl" : "ticket_m"}
              title={sub === "videos" ? "Клипы" : "Концерты"}
              text={`${sourceLabel(source)} не предоставляет эти данные в Mixora. На странице источника может быть больше информации.`}
              action={
                entity.permalink && (
                  <a
                    className="secondary"
                    href={entity.permalink}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Страница исполнителя
                  </a>
                )
              }
            />
          ) : (
            <>
              <div className="library-toolbar">
                <input
                  aria-label="Найти в списке треков"
                  placeholder="Поиск по трекам"
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                />
                <select
                  aria-label="Порядок треков"
                  value={sort}
                  onChange={(e) => setSort(e.target.value)}
                >
                  <option value="default">Порядок по умолчанию</option>
                  <option value="title">По названию</option>
                  <option value="duration">По длительности</option>
                </select>
              </div>
              <TrackList
                tracks={displayTracks}
                playlistId={personal?.id}
                numbered
                emptyText={
                  personal
                    ? "Добавьте музыку через меню трека в поиске."
                    : "Для этого раздела пока нет треков."
                }
              />
            </>
          )}
          {(artist || label) && !sub && !!entity.playlists?.length && (
            <Section
              title="Альбомы и плейлисты"
              to={`${route}/discography?id=${encodeURIComponent(id)}`}
            >
              <PlaylistGrid items={entity.playlists.slice(0, 6)} />
            </Section>
          )}
        </LoadState>
      </div>
      {modal === "about" && (
        <Modal title={title} onClose={() => setModal(null)}>
          <p className="description">
            {entity.description || "Описание не добавлено."}
          </p>
          {trackPage && (
            <p className="muted">
              {entity.genre} · {duration(entity.duration)}
            </p>
          )}
        </Modal>
      )}
      {modal === "edit" && (
        <Modal title="Редактировать плейлист" onClose={() => setModal(null)}>
          <form
            className="auth-form"
            onSubmit={(e) => {
              e.preventDefault();
              if (draft.name.trim()) {
                app.editPlaylist(id, { ...draft, name: draft.name.trim() });
                setModal(null);
              }
            }}
          >
            <label>
              Название
              <input
                required
                maxLength={120}
                value={draft.name}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, name: e.target.value }))
                }
              />
            </label>
            <label>
              Описание
              <textarea
                maxLength={2000}
                value={draft.description}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, description: e.target.value }))
                }
              />
            </label>
            <button className="primary">Сохранить</button>
          </form>
          <div className="playlist-editor">
            {personal.tracks.map((track, i) => (
              <div key={track.id}>
                <span>{track.title}</span>
                <IconButton
                  icon="arrowLeft_xs"
                  label={`Поднять ${track.title}`}
                  disabled={!i}
                  onClick={() => {
                    const list = [...personal.tracks];
                    [list[i - 1], list[i]] = [list[i], list[i - 1]];
                    app.editPlaylist(id, { tracks: list });
                  }}
                />
                <IconButton
                  icon="bucket_xxs"
                  label={`Удалить ${track.title}`}
                  onClick={() => app.removeFromPlaylist(id, track)}
                />
              </div>
            ))}
          </div>
        </Modal>
      )}
      {modal === "delete" && (
        <Modal title="Удалить плейлист?" onClose={() => setModal(null)}>
          <p>«{personal.name}» будет удалён из вашей библиотеки.</p>
          <button
            className="danger"
            onClick={() => {
              if (app.deletePlaylist(id)) {
                navigate("/collection/playlists");
              }
            }}
          >
            Удалить
          </button>
        </Modal>
      )}
    </div>
  );
}
