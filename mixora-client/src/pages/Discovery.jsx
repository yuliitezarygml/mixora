import { Link, useLocation, useSearchParams } from "react-router-dom";
import { useApp } from "../state/context.js";
import { useRemote } from "../lib/useRemote.js";
import { searchCatalog } from "../lib/api.js";
import { genres, moods, podcasts, kids, topicQuery } from "../data/topics.js";
import { TrackList, TrackCards } from "../components/Tracks.jsx";
import {
  ArtistCards,
  PlaylistGrid,
  LoadState,
} from "../components/Catalog.jsx";
import { Section, Tabs, Empty } from "../components/Primitives.jsx";
import Icon from "../components/Icon.jsx";

function catalogKind(type) {
  if (type === "artists") return "users";
  if (type === "albums" || type === "playlists") return "playlists";
  return "tracks";
}

export default function Discovery() {
  const app = useApp();
  const { pathname } = useLocation();
  const [params] = useSearchParams();
  const name = params.get("name") || params.get("tag") || "";
  const isKids = pathname.startsWith("/kids");
  const isPodcasts =
    pathname.startsWith("/non-music") || pathname.includes("/podcasts");
  const isChart = pathname.startsWith("/chart");
  const isMixes = pathname === "/mixes";
  const isTag = pathname === "/tag";
  const type = pathname.endsWith("/albums")
    ? "albums"
    : pathname.endsWith("/playlists") || pathname === "/playlists"
      ? "playlists"
      : pathname.endsWith("/artists")
        ? "artists"
        : "tracks";
  const category = isKids
    ? kids
    : isPodcasts
      ? podcasts
      : isMixes
        ? moods
        : genres;
  const base = isKids
    ? "/kids/category"
    : isPodcasts
      ? "/non-music/category"
      : "/genre";
  const title =
    name ||
    (isKids
      ? "Детям"
      : isPodcasts
        ? isChart
          ? "Чарт подкастов"
          : "Подкасты и книги"
        : isChart
          ? "Чарт"
          : isMixes
            ? "Настроения и занятия"
            : pathname === "/playlists"
              ? "Плейлисты"
              : isTag
                ? "Тема"
                : "Жанры");
  const query =
    (name ? topicQuery(name) : "") ||
    (isKids
      ? "детская музыка"
      : isPodcasts
        ? "подкаст"
        : isChart
          ? "popular music"
          : pathname === "/playlists"
            ? "indie electronic"
            : "");
  const showCategories =
    !name && !isChart && pathname !== "/playlists" && !isTag;
  const remote = useRemote(
    `${app.user?.id}:discovery:${type}:${query}`,
    (signal) => searchCatalog(catalogKind(type), query, signal),
    Boolean(app.user && query),
  );
  const needle = name ? topicQuery(name).split(" ")[0].toLowerCase() : "";
  const fallback =
    app.user || type !== "tracks"
      ? []
      : name
        ? app.catalog.filter((track) =>
            `${track.genre} ${track.title} ${track.artist}`
              .toLowerCase()
              .includes(needle),
          )
        : isChart
          ? app.catalog
          : [];
  let tracks = type === "tracks" ? remote.data || fallback : [];
  if (isKids) tracks = tracks.filter((track) => !track.explicit);
  if (isChart && !isPodcasts) {
    tracks = [...tracks].sort(
      (a, b) => (b.playbackCount || 0) - (a.playbackCount || 0),
    );
  }
  const lists =
    type === "albums" || type === "playlists"
      ? (remote.data || []).filter((item) => type !== "albums" || item.album)
      : [];
  const artists = type === "artists" ? remote.data || [] : [];
  const root = isKids
    ? "KidsPage_root__yycsJ"
    : isPodcasts
      ? "NonMusicPage_root__IPKkH"
      : isChart
        ? "ChartTracksPage_root__QMbqY"
        : isMixes
          ? "MixesPage_root__mp_Eq"
          : "GenrePage_root___kL_v";
  const tabItems = [
    ["Треки", base],
    ["Плейлисты", `${base}/playlists`],
    ["Альбомы", `${base}/albums`],
    ...(!isKids && !isPodcasts ? [["Исполнители", `${base}/artists`]] : []),
  ]
    .filter(
      ([, to]) =>
        !(isKids && (to.endsWith("/albums") || to.endsWith("/playlists"))) &&
        !(isPodcasts && to.endsWith("/playlists")),
    )
    .map(([label, to]) => ({
      label,
      to: `${to}?name=${encodeURIComponent(name)}`,
      active: pathname === to,
    }));

  return (
    <div className={`page-padding migrated-page discovery-page ${root}`}>
      <div className="page-heading">
        <h1>{title}</h1>
        {query && !isPodcasts && (
          <button
            className="secondary"
            onClick={() => app.startWave({ genre: query, title })}
          >
            <Icon name="vibe_xxs" />
            Моя волна
          </button>
        )}
      </div>
      {isChart && !isPodcasts && (
        <p className="muted">
          Популярное среди результатов SoundCloud по числу прослушиваний.
        </p>
      )}
      {name && !isTag && <Tabs items={tabItems} />}
      {showCategories && (
        <Section
          title={
            isMixes
              ? "Ваша музыка на каждый день"
              : isPodcasts
                ? "Категории"
                : isKids
                  ? "Выберите, что послушать"
                  : "Жанры"
          }
        >
          <div className={isMixes ? "mood-grid" : "genre-grid"}>
            {category.map(([label], index) => (
              <Link
                className={`${isMixes ? "mood-card" : "genre-card"} tone-${index % 6}`}
                key={label}
                to={`${base}?name=${encodeURIComponent(label)}`}
              >
                {label}
                <Icon name="arrowRight_xs" />
              </Link>
            ))}
          </div>
        </Section>
      )}
      {isTag && !name && (
        <Empty
          icon="search_xxl"
          title="Тема не выбрана"
          text="Откройте жанр или найдите музыку по названию."
          action={
            <Link className="primary" to="/genre">
              К жанрам
            </Link>
          }
        />
      )}
      {query && (
        <LoadState
          remote={remote}
          source="soundcloud"
          empty={
            type === "artists"
              ? !artists.length
              : type === "albums" || type === "playlists"
                ? !lists.length
                : !tracks.length
          }
          emptyTitle="В этой подборке пока нет доступной музыки"
        >
          {type === "artists" ? (
            <ArtistCards items={artists} />
          ) : type === "albums" || type === "playlists" ? (
            <PlaylistGrid items={lists} />
          ) : (
            <Section
              title={
                name ||
                (isPodcasts
                  ? "Слушать сейчас"
                  : isChart
                    ? "Треки"
                    : "Выбор музыки")
              }
            >
              {isChart && !isPodcasts ? (
                <TrackList tracks={tracks} numbered />
              ) : (
                <>
                  <TrackCards tracks={tracks.slice(0, 6)} />
                  <TrackList tracks={tracks.slice(6)} />
                </>
              )}
            </Section>
          )}
        </LoadState>
      )}
      {!query && showCategories && (
        <Section title="Музыка для вас">
          <TrackCards tracks={app.catalog.slice(0, 6)} />
        </Section>
      )}
    </div>
  );
}
