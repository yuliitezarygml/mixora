import { Link } from "react-router-dom";
import { useApp } from "../state/context.js";
import { TrackCards } from "../components/Tracks.jsx";
import { Section } from "../components/Primitives.jsx";
import Icon from "../components/Icon.jsx";
import VibeBackground from "../components/VibeBackground.jsx";
import { Cover } from "../components/Primitives.jsx";
import { waveExplanation } from "../lib/wave.js";
export default function Home() {
  const app = useApp();
  return (
    <>
      <section className="vibe VibeBlock_root__z7LtR MainPage_vibe__XEBbh">
        <VibeBackground />
        <div className="vibe-content VibeBlock_controls__BpDFL">
          <h1 className="wave-title">Моя волна</h1>
          {app.waveActive && app.current && (
            <div className="wave-now">
              <Cover track={app.current} />
              <p>
                {app.current.artist} — {app.current.title}
              </p>
            </div>
          )}
          <button
            className="vibe-play PlayButton_root__nYKdN"
            onClick={() =>
              app.waveActive && app.current ? app.toggle() : app.startWave()
            }
            disabled={app.waveBusy}
            aria-busy={app.waveBusy}
          >
            <Icon
              name={
                app.waveActive && app.playing ? "pause_filled_l" : "playVibe_s"
              }
              size={32}
            />
            <span>
              {app.waveBusy
                ? "Настраиваем волну…"
                : app.waveActive && app.playing
                  ? "Пауза"
                  : "Слушать"}
            </span>
          </button>
          <button
            className="vibe-settings VibeBlock_settingsButton__GeMtO"
            onClick={() => app.setWaveSettingsOpen(true)}
          >
            <Icon name="filter_xs" size={20} />
            Настроить мою волну
          </button>
          {app.waveActive && app.current && (
            <p className="wave-note">
              <Icon name="vibe_xxs" size={16} />
              {waveExplanation(app.waveModelVersion)}
            </p>
          )}
        </div>
      </section>
      <div className="page-padding MainPage_landing___FGNm home-landing">
        <Section title="Ваша музыка" to="/collection">
          <div className="quick-cards">
            <Link
              className="quick-card favorites"
              to="/mymusic/favorite_tracks"
            >
              <div>
                <Icon name="liked_m" size={36} />
              </div>
              <span>
                Мне нравится<small>{app.library.likes.length} треков</small>
              </span>
            </Link>
            <Link className="quick-card" to="/music-history">
              <div>
                <Icon name="history_m" size={36} />
              </div>
              <span>
                Недавно слушали<small>Вернитесь к любимому</small>
              </span>
            </Link>
            <Link className="quick-card" to="/collection/playlists">
              <div>
                <Icon name="playlist_xl" size={36} />
              </div>
              <span>
                Мои плейлисты<small>Всё в одном месте</small>
              </span>
            </Link>
          </div>
        </Section>
        <Section title="Откройте для себя" to="/search">
          <TrackCards tracks={app.catalog.slice(0, 6)} />
        </Section>
        <Section title="Настроения и занятия" to="/mixes">
          <div className="mood-grid">
            {["Для работы", "Расслабиться", "В дороге", "Зарядиться"].map(
              (title, i) => (
                <Link
                  key={title}
                  className={`mood-card tone-${i}`}
                  to={`/genre?name=${encodeURIComponent(title)}`}
                >
                  {title}
                  <Icon name="vibe_xxs" size={32} />
                </Link>
              ),
            )}
          </div>
        </Section>
        <Section title="Другой ритм" to="/genre?name=Electronic">
          <TrackCards tracks={app.catalog.slice(10, 16)} />
        </Section>
      </div>
    </>
  );
}
