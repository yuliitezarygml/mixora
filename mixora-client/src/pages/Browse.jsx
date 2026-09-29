import { useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { useApp } from "../state/context.js";
import { TrackList } from "../components/Tracks.jsx";
import { Section, Tabs, Empty } from "../components/Primitives.jsx";
import Icon from "../components/Icon.jsx";
export default function Browse() {
  const a = useApp(),
    { pathname } = useLocation();
  const [city, setCity] = useState("Все города");
  if (pathname === "/music-history")
    return (
      <div className="page-padding">
        <h1>История прослушивания</h1>
        <p className="muted">На этом устройстве</p>
        <TrackList
          tracks={a.library.history}
          emptyText="Послушайте музыку — здесь появятся ваши последние треки."
        />
      </div>
    );
  if (pathname === "/search/history")
    return (
      <div className="page-padding">
        <div className="page-heading">
          <h1>История поиска</h1>
          <button
            className="text-button"
            onClick={() => a.updateLibrary((s) => ({ ...s, searches: [] }))}
          >
            Очистить
          </button>
        </div>
        {a.library.searches.length ? (
          a.library.searches.map((q) => (
            <Link
              className="history-query"
              key={q}
              to={`/search?q=${encodeURIComponent(q)}`}
            >
              <Icon name="history_m" />
              {q}
              <Icon name="arrowRight_xs" />
            </Link>
          ))
        ) : (
          <Empty icon="search_xxl" title="Вы ещё ничего не искали" />
        )}
      </div>
    );
  if (pathname.startsWith("/concert"))
    return (
      <div className="page-padding">
        <div className="page-heading">
          <h1>{pathname === "/concert" ? "Концерт" : "Концерты"}</h1>
          <select
            aria-label="Город концертов"
            value={city}
            onChange={(e) => setCity(e.target.value)}
          >
            {["Все города", "Кишинёв", "Москва", "Санкт-Петербург"].map((c) => (
              <option key={c}>{c}</option>
            ))}
          </select>
        </div>
        <Tabs
          items={[
            { label: "Все концерты", to: "/concerts", active: true },
            { label: "Любимые исполнители", to: "/collection/artists" },
          ]}
        />
        <Empty
          icon="ticket_m"
          title="Афиша скоро появится"
          text={`Пока нет добавленных концертов${city === "Все города" ? "" : ` в городе ${city}`}.`}
        />
      </div>
    );
  if (pathname.startsWith("/video"))
    return (
      <div className="page-padding">
        <h1>Клипы</h1>
        <div className="video-surface">
          <Icon name="clip_xl" size={80} />
          <p>Выберите клип в коллекции</p>
        </div>
        <Section title="Ваша коллекция">
          <Empty
            title="Клипов пока нет"
            text="Источник видео ещё не подключён."
          />
        </Section>
      </div>
    );
  if (pathname === "/post")
    return (
      <div className="page-padding editorial">
        <span className="eyebrow">Музыкальная редакция</span>
        <h1>Откройте для себя новую музыку</h1>
        <img
          className="editorial-image"
          src="/assets/media/vibe_animation_fallback/vibe_animation_fallback_dark.jpeg"
          alt="Анимация музыкальной волны"
        />
        <p>
          Начните с подборки треков и сохраните понравившиеся в своей коллекции.
        </p>
        <TrackList tracks={a.catalog.slice(0, 5)} />
      </div>
    );
  if (pathname.startsWith("/rewind") || pathname.startsWith("/slides"))
    return (
      <div className="year-page">
        <span className="eyebrow">Ваш год в музыке</span>
        <h1>
          Музыка,
          <br />
          которая с вами
        </h1>
        <p>Ваши любимые треки и исполнители</p>
        <div className="year-stat">
          <strong>{a.library.history.length}</strong>
          <span>треков в истории</span>
        </div>
        {a.library.history.length ? (
          <button
            className="primary"
            onClick={() => a.play(a.library.history[0], a.library.history)}
          >
            Слушать историю
          </button>
        ) : (
          <Link className="primary" to="/">
            Начать слушать
          </Link>
        )}
        <Link to="/music-history">Перейти к истории</Link>
      </div>
    );
  if (pathname === "/entities")
    return (
      <div className="page-padding">
        <h1>Все разделы</h1>
        <div className="genre-grid">
          {[
            ["Главная", "/"],
            ["Поиск", "/search"],
            ["Коллекция", "/collection"],
            ["Чарт", "/chart"],
            ["Настроения", "/mixes"],
            ["Детям", "/kids"],
            ["История", "/music-history"],
            ["Настройки", "/settings"],
            ["Мой год", "/rewind2024"],
            ["Редакция", "/post"],
          ].map(([label, to], i) => (
            <Link key={to} to={to} className={`genre-card tone-${i % 6}`}>
              {label}
            </Link>
          ))}
        </div>
      </div>
    );
  if (pathname.startsWith("/users"))
    return (
      <div className="page-padding">
        <div className="profile-hero">
          <div className="avatar big">{a.user?.display_name?.[0] || "М"}</div>
          <div>
            <span className="eyebrow">Слушатель</span>
            <h1>{a.user?.display_name || "Ваша музыка"}</h1>
            <p className="muted">
              {a.library.playlists.length} плейлистов · {a.library.likes.length}{" "}
              любимых треков
            </p>
          </div>
        </div>
        <Tabs
          items={[
            { label: "Обзор", to: "/users", active: pathname === "/users" },
            {
              label: "Плейлисты",
              to: "/users/playlists",
              active: pathname.includes("playlists"),
            },
          ]}
        />
        <div className="card-grid">
          {a.library.playlists.map((p) => (
            <Link className="music-card" key={p.id} to={`/playlist?id=${p.id}`}>
              <div className="playlist-cover">
                <Icon name="playlist_xl" size={64} />
              </div>
              <strong>{p.name}</strong>
              <span className="muted">{p.tracks.length} треков</span>
            </Link>
          ))}
        </div>
        {!a.library.playlists.length && (
          <Empty
            title="Плейлистов пока нет"
            action={
              <Link className="primary" to="/collection/playlists">
                Создать первый
              </Link>
            }
          />
        )}
      </div>
    );
  return (
    <div className="page-padding">
      <Empty
        title="Раздел не подключён"
        text="Этот экран исходного клиента не связан с каталогом Mixora."
        action={
          <Link className="primary" to="/">
            На главную
          </Link>
        }
      />
    </div>
  );
}
