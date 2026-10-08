import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useApp } from "../state/context.js";
import { duration, trackKey, uniqueTracks } from "../lib/library.js";
import {
  musicSources,
  resolveExternalTrack,
  searchCatalog,
  searchSpotifyCatalog,
  searchYouTubeCatalog,
} from "../lib/api.js";
import { correctQuery } from "../lib/suggest.js";
import { useRemote } from "../lib/useRemote.js";
import { TrackList, TrackCards } from "../components/Tracks.jsx";
import {
  ArtistCards,
  PlaylistGrid,
  LoadState,
} from "../components/Catalog.jsx";
import { Section, Empty, IconButton } from "../components/Primitives.jsx";
import Icon from "../components/Icon.jsx";
export default function Search() {
  const app = useApp(),
    [params, setParams] = useSearchParams();
  const q = params.get("q") || "",
    kind = params.get("type") || "all",
    source = params.get("source") || "soundcloud";
  const sourceDefinition =
    musicSources.find((item) => item.id === source) || musicSources[0];
  const supportedKinds = sourceDefinition.kinds;
  const [input, setInput] = useState(q);
  const [shelf, setShelf] = useState("popular");
  const recordedSearch = useRef("");
  useEffect(() => setInput(q), [q]);
  useEffect(() => {
    if (input.trim() === q) return;
    const timer = setTimeout(
      () =>
        setParams(
          (p) => {
            const next = new URLSearchParams(p);
            if (input.trim()) next.set("q", input.trim());
            else next.delete("q");
            return next;
          },
          { replace: true },
        ),
      450,
    );
    return () => clearTimeout(timer);
  }, [input, q, setParams]);
  const remote = useRemote(
    `${app.user?.id}:search:${source}:${kind}:${q}:${source === "local" ? app.catalog.length : ""}`,
    async (signal) => {
      if (source === "local") {
        const normalized = q.toLocaleLowerCase();
        const tracks = app.catalog.filter((track) =>
          `${track.title} ${track.artist}`
            .toLocaleLowerCase()
            .includes(normalized),
        );
        return {
          tracks,
          artists:
            kind === "all" || kind === "artists"
              ? [
                  ...new Map(
                    tracks
                      .filter((track) => track.artistId)
                      .map((track) => [
                        `${track.source}:${track.artistId}`,
                        {
                          id: track.artistId,
                          source: track.source,
                          name: track.artist,
                          artwork: track.artwork,
                        },
                      ]),
                  ).values(),
                ]
              : [],
          playlists:
            kind === "all" || kind === "playlists" || kind === "albums"
              ? [
                  ...app.library.playlists,
                  ...app.library.savedPlaylists,
                ].filter((playlist) =>
                  playlist.name.toLocaleLowerCase().includes(normalized),
                )
              : [],
        };
      }
      if (source === "spotify") {
        const requestedKinds =
          kind === "all"
            ? ["tracks", "artists", "albums", "playlists"]
            : [kind];
        const entries = await Promise.allSettled(
          requestedKinds.map((value) => searchSpotifyCatalog(value, q, signal)),
        );
        if (entries.every((entry) => entry.status === "rejected")) {
          throw entries[0].reason;
        }
        const result = Object.fromEntries(
          requestedKinds.map((value, index) => [
            value,
            entries[index].status === "fulfilled" ? entries[index].value : [],
          ]),
        );
        return {
          tracks: result.tracks || [],
          artists: result.artists || [],
          playlists: [...(result.albums || []), ...(result.playlists || [])],
        };
      }
      if (source === "youtube") {
        return {
          tracks:
            kind === "all" || kind === "tracks"
              ? await searchYouTubeCatalog(q, signal)
              : [],
          artists: [],
          playlists: [],
        };
      }
      if (source === "external") {
        return {
          tracks:
            kind === "all" || kind === "tracks"
              ? [await resolveExternalTrack(q, signal)]
              : [],
          artists: [],
          playlists: [],
        };
      }
      const entries = await Promise.allSettled([
        kind === "all" || kind === "tracks"
          ? searchCatalog("tracks", q, signal)
          : [],
        kind === "all" || kind === "artists"
          ? searchCatalog("users", q, signal)
          : [],
        kind === "all" || kind === "playlists" || kind === "albums"
          ? searchCatalog("playlists", q, signal)
          : [],
      ]);
      if (entries.every((x) => x.status === "rejected"))
        throw entries[0].reason;
      const [tracks, artists, playlists] = entries.map((x) =>
        x.status === "fulfilled" ? x.value : [],
      );
      if (kind !== "all") {
        const i = kind === "tracks" ? 0 : kind === "artists" ? 1 : 2;
        if (entries[i].status === "rejected") throw entries[i].reason;
      }
      return { tracks, artists, playlists };
    },
    !!q && (!!app.user || source === "local"),
  );
  useEffect(() => {
    if (remote.data?.tracks.length)
      app.setCatalog((previous) =>
        uniqueTracks([...previous, ...remote.data.tracks]),
      );
  }, [remote.data]);
  useEffect(() => {
    if (!q || !remote.data) return;
    const visibleTracks =
      kind === "all"
        ? remote.data.tracks.slice(0, 6)
        : kind === "tracks"
          ? remote.data.tracks.slice(0, 40)
          : [];
    const key = `${app.user?.id || "guest"}:${source}:${kind}:${q}:${visibleTracks
      .map((track) => `${track.source}:${track.id}`)
      .join(",")}`;
    if (recordedSearch.current === key) return;
    recordedSearch.current = key;
    app.recordSearch(q, visibleTracks, {
      kind,
      source,
      resultCount: remote.data.tracks.length,
    });
  }, [app.user?.id, kind, q, remote.data, source]);
  // A remote source must never fall back to a mixed local cache: otherwise a
  // guest selecting Spotify/YouTube could see and play unrelated SoundCloud
  // rows. Only the explicit Mixora-local source renders cached results.
  const local =
    source === "local"
      ? app.catalog.filter((track) =>
          `${track.title} ${track.artist}`
            .toLowerCase()
            .includes(q.toLowerCase()),
        )
      : [];
  const data =
    remote.data ||
    (source === "local"
      ? {
          tracks: local,
          artists: [
            ...new Map(
              local.map((track) => [
                `${track.source}:${track.artistId || track.artist}`,
                {
                  id: track.artistId,
                  source: track.source,
                  name: track.artist,
                  artwork: track.artwork,
                },
              ]),
            ).values(),
          ],
          playlists: [
            ...app.library.playlists,
            ...app.library.savedPlaylists,
          ].filter((playlist) =>
            playlist.name.toLowerCase().includes(q.toLowerCase()),
          ),
        }
      : { tracks: [], artists: [], playlists: [] });
  const setFilter = (key, value) =>
    setParams((previous) => {
      const next = new URLSearchParams(previous);
      next.set(key, value);
      return next;
    });
  const setSource = (nextSource) => {
    const nextDefinition = musicSources.find((item) => item.id === nextSource);
    setParams((previous) => {
      const next = new URLSearchParams(previous);
      next.set("source", nextSource);
      if (!nextDefinition?.kinds.includes(kind)) next.set("type", "all");
      return next;
    });
  };
  const sourceSelector = (
    <select
      aria-label="Источник поиска"
      value={source}
      onChange={(event) => setSource(event.target.value)}
    >
      {musicSources.map((item) => (
        <option key={item.id} value={item.id}>
          {item.label}
        </option>
      ))}
    </select>
  );
  const submit = (e) => {
    e.preventDefault();
    const value = input.trim();
    setFilter("q", value);
    if (value)
      app.updateLibrary((s) => ({
        ...s,
        searches: [
          { kind: "query", query: value, title: value, source },
          ...s.searches.filter(
            (item) =>
              (typeof item === "string" ? item : item.query) !== value ||
              (typeof item === "string"
                ? "soundcloud"
                : item.source || "soundcloud") !== source,
          ),
        ].slice(0, 30),
      }));
  };
  const samples = [
    ...app.catalog.flatMap((track) => [track.title, track.artist]),
    ...app.library.searches.map((item) =>
      typeof item === "string" ? item : item.query,
    ),
  ];
  const hint = q ? correctQuery(q, samples) : "";
  const corrected = hint.toLocaleLowerCase() !== q.toLocaleLowerCase();
  const rememberTrack = (track) =>
    app.updateLibrary((state) => ({
      ...state,
      searches: [
        {
          kind: "track",
          query: track.title,
          title: track.title,
          track,
        },
        ...state.searches.filter(
          (item) => !item?.track || trackKey(item.track) !== trackKey(track),
        ),
      ].slice(0, 30),
    }));
  const collections = [
    ["Осенняя", "Acoustic"],
    ["Настроения", "Ambient"],
    ["Занятия", "Electronic"],
    ["Жанры", "Hip Hop"],
    ["Эпохи", "Rock"],
  ].map(([title, genre], index) => ({
    title,
    genre,
    artwork: app.catalog[index]?.artwork,
  }));
  return (
    <div className="page-padding search-page SearchPage_root__TtwTi">
      <form className="search-box" onSubmit={submit}>
        <Icon name="search_m" />
        <input
          autoFocus
          aria-label="Поиск музыки"
          placeholder={
            source === "external"
              ? "Вставьте ссылку YouTube, VK, Bandcamp…"
              : "Что вы чувствуете или ищете?"
          }
          value={input}
          onChange={(e) => setInput(e.target.value)}
        />
        {input && (
          <IconButton
            icon="close_xxs"
            label="Очистить поиск"
            type="button"
            onClick={() => {
              setInput("");
              setParams((previous) => {
                const next = new URLSearchParams(previous);
                next.delete("q");
                next.delete("type");
                return next;
              });
            }}
          />
        )}
        <button type="submit" className="sr-only">
          Найти
        </button>
      </form>
      {!q && (
        <div className="chip-row" role="group" aria-label="Источник поиска">
          {sourceSelector}
        </div>
      )}
      {q
        ? corrected && (
            <p className="search-hint">
              Возможно, вы искали{" "}
              <button type="button" onClick={() => setInput(hint)}>
                {hint}
              </button>
            </p>
          )
        : null}
      {q ? (
        <>
          <div className="chip-row" role="group" aria-label="Тип результатов">
            {[
              ["all", "Всё"],
              ["tracks", "Треки"],
              ["artists", "Исполнители"],
              ["albums", "Альбомы"],
              ["playlists", "Плейлисты"],
            ]
              .filter(([value]) => supportedKinds.includes(value))
              .map(([value, label]) => (
                <button
                  key={value}
                  className={kind === value ? "selected" : ""}
                  aria-pressed={kind === value}
                  onClick={() => setFilter("type", value)}
                >
                  {label}
                </button>
              ))}
            {sourceSelector}
          </div>
          {!app.user && source !== "local" && (
            <div className="inline-notice">
              Войдите для поиска по подключённым источникам.
              <button onClick={() => app.setAuthOpen(true)}>
                Войти для полного поиска
              </button>
            </div>
          )}
          <LoadState remote={remote} source={source}>
            {(kind === "all" || kind === "tracks") && (
              <Section title="Треки">
                <TrackList
                  tracks={
                    kind === "all" ? data.tracks.slice(0, 6) : data.tracks
                  }
                  onActivate={rememberTrack}
                />
                {kind === "all" && data.tracks.length > 6 && (
                  <button
                    className="text-button"
                    onClick={() => setFilter("type", "tracks")}
                  >
                    Все треки
                  </button>
                )}
              </Section>
            )}
            {(kind === "all" || kind === "artists") &&
              !!data.artists.length && (
                <Section title="Исполнители">
                  <ArtistCards
                    items={
                      kind === "all" ? data.artists.slice(0, 6) : data.artists
                    }
                  />
                </Section>
              )}
            {(kind === "all" || kind === "albums" || kind === "playlists") && (
              <Section title={kind === "albums" ? "Альбомы" : "Плейлисты"}>
                <PlaylistGrid
                  items={data.playlists
                    .filter((p) => kind !== "albums" || p.album)
                    .slice(0, kind === "all" ? 6 : 40)}
                />
              </Section>
            )}
            {kind !== "tracks" &&
              !data.artists.length &&
              !data.playlists.length &&
              !data.tracks.length && (
                <Empty
                  title="Ничего не найдено"
                  text="Попробуйте другое название или имя исполнителя."
                />
              )}
          </LoadState>
        </>
      ) : (
        <>
          <div className="chip-row" role="tablist" aria-label="Полки поиска">
            {[
              ["popular", "Популярное"],
              ["history", "История"],
            ].map(([value, label]) => (
              <button
                key={value}
                role="tab"
                aria-selected={shelf === value}
                className={shelf === value ? "selected" : ""}
                onClick={() => setShelf(value)}
              >
                {label}
              </button>
            ))}
          </div>
          {shelf === "history" ? (
            <section className="search-history">
              <h2>История поиска</h2>
              {app.library.searches.length ? (
                <>
                  <div className="search-history-grid">
                    {app.library.searches.slice(0, 12).map((item, index) => {
                      if (item?.kind === "track" && item.track) {
                        const track = item.track;
                        const liked = app.library.likes.some(
                          (entry) => trackKey(entry) === trackKey(track),
                        );
                        return (
                          <div
                            className="search-history-row"
                            key={`track-${track.id}-${index}`}
                          >
                            <button
                              onClick={() =>
                                app.play(
                                  track,
                                  app.library.searches
                                    .filter((entry) => entry?.track)
                                    .map((entry) => entry.track),
                                )
                              }
                            >
                              <img src={track.artwork} alt="" />
                              <span>
                                <strong>{track.title}</strong>
                                <small>{track.artist}</small>
                              </span>
                            </button>
                            <IconButton
                              icon={liked ? "liked_xs" : "like_xs"}
                              label={
                                liked
                                  ? `Убрать из любимого: ${track.title}`
                                  : `Нравится: ${track.title}`
                              }
                              active={liked}
                              onClick={() => app.toggleLike(track)}
                            />
                            <em>{duration(track.duration)}</em>
                          </div>
                        );
                      }
                      const text = typeof item === "string" ? item : item.title;
                      const query =
                        typeof item === "string" ? item : item.query || text;
                      const querySource =
                        typeof item === "string"
                          ? "soundcloud"
                          : item.source || "soundcloud";
                      return (
                        <Link
                          key={`query-${query}-${index}`}
                          to={`/search?q=${encodeURIComponent(query)}&source=${encodeURIComponent(querySource)}`}
                        >
                          <Icon name="search_m" />
                          <span>
                            <strong>{text}</strong>
                            <small>Запрос</small>
                          </span>
                        </Link>
                      );
                    })}
                  </div>
                  <button
                    className="search-clear"
                    onClick={() =>
                      app.updateLibrary((s) => ({ ...s, searches: [] }))
                    }
                  >
                    Очистить историю
                  </button>
                </>
              ) : (
                <Empty
                  icon="search_xxl"
                  title="Вы ещё ничего не искали"
                  text="Запросы и прослушанные треки появятся здесь."
                />
              )}
            </section>
          ) : (
            <>
              <Section title="Подборки музыки" to="/mixes">
                <div className="collection-folders">
                  {collections.map((item) => (
                    <Link
                      key={item.title}
                      to={`/genre?name=${encodeURIComponent(item.genre)}`}
                    >
                      {item.artwork ? (
                        <img src={item.artwork} alt="" />
                      ) : (
                        <span />
                      )}
                      {item.title}
                    </Link>
                  ))}
                </div>
              </Section>
              <Section title="Вы могли пропустить" to="/search">
                <TrackCards
                  tracks={app.catalog.slice(0, 6)}
                  onActivate={rememberTrack}
                />
              </Section>
            </>
          )}
        </>
      )}
    </div>
  );
}
