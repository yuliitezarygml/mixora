import { useState } from "react";
import { useApp } from "../state/context.js";
import { waveExplanation } from "../lib/wave.js";
import { filterRecommendedTracks } from "../lib/recommendedPlaylists.js";
import { Modal } from "./Primitives.jsx";
import { TrackList } from "./Tracks.jsx";
import Icon from "./Icon.jsx";

const mixArtwork = {
  daily: { theme: "dark", label: "Под ваш вкус", icon: "liked_m" },
  discover: { theme: "light", label: "Новая музыка", icon: "vibe_xxs" },
  focus: { theme: "dark", label: "Спокойный ритм", icon: "note_xl" },
};

function MixCover({ playlist }) {
  const art = mixArtwork[playlist.id] || mixArtwork.daily;
  return (
    <span
      className={`mix-artwork mix-artwork-${playlist.id} mix-artwork-${art.theme}`}
      aria-hidden="true"
    >
      <img
        src={`/assets/media/vibe_animation_fallback/vibe_animation_fallback_${art.theme}.jpeg`}
        alt=""
        width="1000"
        height="1000"
      />
      <span className="mix-artwork-brand">MIXORA</span>
      <Icon className="mix-artwork-icon" name={art.icon} size={40} />
      <span className="mix-artwork-label">{art.label}</span>
    </span>
  );
}

export default function RecommendedPlaylists({ collection = false }) {
  const app = useApp();
  const { playlists, loading, failures, refresh } = app.recommendations;
  const [selected, setSelected] = useState(null);
  const [saved, setSaved] = useState(false);
  const current =
    selected && app.user && selected.owner === app.user.id
      ? {
          ...selected,
          tracks: filterRecommendedTracks(
            selected.tracks,
            app.library,
            app.settings.explicit,
            selected.id === "discover",
          ),
        }
      : null;
  const open = (playlist) => {
    setSaved(false);
    setSelected(playlist);
    app.rememberRecommendedPlaylist(playlist);
  };
  return (
    <section
      className={`recommended-playlists ${collection ? "recommended-in-collection" : ""}`}
      aria-label="Рекомендуемые плейлисты"
      aria-busy={loading}
    >
      <header className="section-heading">
        <h2>Рекомендуемые плейлисты</h2>
        {app.user && (
          <button
            className="recommendation-refresh secondary"
            disabled={loading}
            onClick={refresh}
          >
            <Icon name="repeat_xs" size={18} />
            Обновить
          </button>
        )}
      </header>
      <p className="muted recommendation-note">
        Подстраиваются под ваши лайки, историю и поиски.
      </p>
      {!app.user ? (
        <button className="text-button" onClick={() => app.setAuthOpen(true)}>
          Войти для персональных подборок
        </button>
      ) : loading ? (
        <p role="status" className="muted">
          Собираем музыку для вас…
        </p>
      ) : playlists.length ? (
        <div className="recommended-grid">
          {playlists.map((playlist) => (
            <article className="recommended-card" key={playlist.id}>
              <div className="recommended-cover">
                <button
                  className="recommended-open"
                  aria-label={`Посмотреть плейлист «${playlist.name}»`}
                  onClick={() => open(playlist)}
                >
                  <MixCover playlist={playlist} />
                </button>
                <button
                  className="card-play"
                  aria-label={`Слушать подборку «${playlist.name}»`}
                  onClick={() => app.playRecommendedPlaylist(playlist)}
                >
                  <Icon name="play_filled_l" size={28} />
                </button>
              </div>
              <button
                className="recommended-title"
                onClick={() => open(playlist)}
              >
                {playlist.name}
              </button>
              <span className="muted">{playlist.description}</span>
              <small className="muted">
                {playlist.tracks.length} треков ·{" "}
                {playlist.modelVersion === "rules-v0"
                  ? "Базовая подборка"
                  : "Персональная подборка"}
              </small>
              {playlist.limitedCatalog && (
                <small
                  className="muted"
                  title="Подборка пересекается с «Для вас»: в каталоге пока мало подходящей музыки."
                >
                  Мало подходящей музыки
                </small>
              )}
            </article>
          ))}
        </div>
      ) : (
        <p role="status" className="muted">
          {failures
            ? "Не удалось загрузить подборки. Попробуйте обновить."
            : "Пока нет доступных треков для этих настроек. Попробуйте другой язык или добавьте музыку в любимое."}
        </p>
      )}
      {!loading && failures > 0 && playlists.length > 0 && (
        <p role="status" className="muted">
          Часть подборок временно недоступна. Можно обновить.
        </p>
      )}
      {current && (
        <Modal title={current.name} onClose={() => setSelected(null)}>
          {current.limitedCatalog && (
            <p className="muted">
              В каталоге пока мало музыки для этих настроек: часть треков
              совпадает с подборкой «Для вас».
            </p>
          )}
          <p className="muted">
            {waveExplanation(current.modelVersion)}. Это снимок текущей
            подборки; сохранённый плейлист не изменяется автоматически.
          </p>
          <div className="recommendation-actions">
            <button
              className="primary"
              disabled={!current.tracks.length}
              onClick={() => app.playRecommendedPlaylist(current)}
            >
              Слушать подборку
            </button>
            <button
              className="secondary"
              disabled={saved || !current.tracks.length}
              onClick={() => {
                const playlist = app.createPlaylist(
                  current.name,
                  current.tracks,
                );
                if (playlist) {
                  setSaved(true);
                  app.toast("Подборка сохранена в ваши плейлисты");
                }
              }}
            >
              {saved ? "Сохранено в коллекцию" : "Сохранить в мои плейлисты"}
            </button>
          </div>
          <TrackList
            tracks={current.tracks}
            compact
            showSourceInMetadata
            onPlay={(track) => app.playRecommendedPlaylist(current, track)}
          />
        </Modal>
      )}
    </section>
  );
}
