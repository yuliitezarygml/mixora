import { useState } from "react";
import { useApp } from "../state/context.js";
import { duration, trackKey } from "../lib/library.js";
import Icon from "./Icon.jsx";
import { Cover, IconButton } from "./Primitives.jsx";
import FullscreenPlayer from "./FullscreenPlayer.jsx";
import SoundSettings from "./SoundSettings.jsx";
import { TrackMenu } from "./Tracks.jsx";
export default function Player() {
  const a = useApp();
  const [soundOpen, setSoundOpen] = useState(false);
  const [menuTrack, setMenuTrack] = useState(null);
  const liked =
    a.current &&
    a.library.likes.some((t) => trackKey(t) === trackKey(a.current));
  const canSeek =
    Boolean(a.current) && Number.isFinite(a.length) && a.length > 0;
  const seekPosition =
    canSeek && Number.isFinite(a.position)
      ? Math.max(0, Math.min(a.position, a.length))
      : 0;
  const progress = canSeek ? (seekPosition / a.length) * 100 : 0;
  return (
    <footer
      className="player CommonLayout_playerBar__zXRxq PlayerBarDesktop_root__d2Hwi"
      aria-label="Музыкальный плеер"
      style={{ "--progress": `${progress}%` }}
    >
      <div className="player-progress">
        <input
          aria-label="Позиция воспроизведения"
          aria-valuetext={`${duration(a.position)} из ${duration(a.length)}`}
          type="range"
          min="0"
          max={canSeek ? a.length : 1}
          value={seekPosition}
          step=".1"
          disabled={!canSeek}
          onChange={(e) => a.seek(Number(e.target.value))}
        />
      </div>
      <div className="player-track PlayerBarDesktop_info__56v53">
        <Cover track={a.current} />
        <div className="player-track-text">
          <div className="player-title-row">
            <strong>{a.current?.title || "Выберите музыку"}</strong>
            {a.current && (
              <IconButton
                icon="more_xxs"
                label="Действия с треком"
                onClick={() => setMenuTrack(a.current)}
              />
            )}
          </div>
          {a.current ? (
            <span>{a.current.artist}</span>
          ) : (
            <span>Треки, которые хочется слушать</span>
          )}
        </div>
      </div>
      <div className="player-center PlayerBarDesktop_sonata__sJHY_">
        <div className="player-buttons BaseSonataControlsDesktop_root__E6wjA SonataControls_root__w8uqu">
          <IconButton
            icon="dislike_xs"
            label="Не нравится текущий трек"
            onClick={() => a.dislike(a.current)}
            disabled={!a.current}
          />
          <IconButton
            icon="shuffle_xs"
            label="Перемешать"
            active={a.shuffled}
            onClick={a.toggleShuffle}
            disabled={!a.current}
          />
          <div className="player-transport">
            <IconButton
              icon="previous_xs"
              label="Предыдущий трек"
              onClick={a.previous}
              disabled={!a.current}
            />
            <button
              className="main-play"
              aria-label={a.playing ? "Приостановить" : "Воспроизвести"}
              onClick={() =>
                a.current ? a.toggle() : a.play(a.catalog[0], a.catalog)
              }
              disabled={!a.current && !a.catalog.length}
              aria-busy={a.loading}
            >
              <Icon
                name={a.playing ? "pause_filled_l" : "play_filled_l"}
                size={40}
              />
            </button>
            <IconButton
              icon="next_xs"
              label="Следующий трек"
              onClick={() => a.next()}
              disabled={!a.current}
            />
          </div>
          <IconButton
            icon={a.repeat === "one" ? "repeat_one_xs" : "repeat_xs"}
            label={`Повтор: ${a.repeat === "off" ? "выключен" : a.repeat === "all" ? "все" : "один"}`}
            active={a.repeat !== "off"}
            onClick={() =>
              a.setRepeat(
                a.repeat === "off" ? "all" : a.repeat === "all" ? "one" : "off",
              )
            }
          />
          <IconButton
            icon={liked ? "liked_xs" : "like_xs"}
            label="Нравится текущий трек"
            active={liked}
            onClick={() => a.toggleLike(a.current)}
            disabled={!a.current}
          />
        </div>
      </div>
      <div className="player-tools PlayerBarDesktop_meta__6sm58">
        <IconButton
          icon="syncLyrics_xs"
          label="Текст трека"
          title=""
          active={a.panel === "lyrics"}
          onClick={() => a.setPanel(a.panel === "lyrics" ? null : "lyrics")}
        />
        <IconButton
          icon="playQueue_xs"
          label="Очередь воспроизведения"
          title=""
          active={a.panel === "queue"}
          onClick={() => a.setPanel(a.panel === "queue" ? null : "queue")}
        />
        <IconButton
          icon="settings_xs"
          label="Настройки звука"
          title=""
          active={soundOpen}
          onClick={() => setSoundOpen(true)}
        />
        <div className="volume-control">
          <IconButton
            icon={a.settings.volume ? "volume_xs" : "volumeOff_xs"}
            label="Выключить или включить звук"
            title=""
            onClick={() =>
              a.setSettings({ volume: a.settings.volume ? 0 : 0.65 })
            }
          />
          <input
            aria-label="Громкость"
            type="range"
            min="0"
            max="1"
            step=".01"
            value={a.settings.volume}
            onChange={(e) => a.setSettings({ volume: Number(e.target.value) })}
          />
        </div>
      </div>
      {a.playbackError && (
        <div className="player-error" role="alert">
          <span>{a.playbackError}</span>
          <button
            className="text-button player-error-retry"
            type="button"
            onClick={a.retryPlayback}
          >
            Повторить
          </button>
        </div>
      )}
      {soundOpen && <SoundSettings onClose={() => setSoundOpen(false)} />}
      {menuTrack && (
        <TrackMenu track={menuTrack} onClose={() => setMenuTrack(null)} />
      )}
    </footer>
  );
}
export function PlayerPanel() {
  const a = useApp();
  if (!a.panel) return null;
  if (a.panel === "fullscreen") return <FullscreenPlayer />;
  return (
    <aside
      className={`player-panel ${a.panel === "fullscreen" ? "fullscreen-player" : ""}`}
    >
      <header>
        <h2>{a.panel === "queue" ? "Очередь" : "Текст трека"}</h2>
        <IconButton
          icon="close_xs"
          label="Закрыть панель плеера"
          onClick={() => a.setPanel(null)}
        />
      </header>
      {a.panel === "queue" ? (
        <>
          <p className="muted">{a.queue.length} треков</p>
          {a.queue.length ? (
            <ol className="queue-list">
              {a.queue.map((t, i) => (
                <li
                  key={`${trackKey(t)}-${i}`}
                  className={a.index === i ? "selected" : ""}
                >
                  <button
                    className="queue-track"
                    aria-label={`${a.index === i && a.playing ? "Приостановить" : "Воспроизвести"}: ${t.title}`}
                    onClick={() => (a.index === i ? a.toggle() : a.setIndex(i))}
                  >
                    <Cover track={t} />
                    <span>
                      <strong>{t.title}</strong>
                      <small>{t.artist}</small>
                    </span>
                    <Icon
                      name={a.index === i && a.playing ? "pause_xs" : "play_xs"}
                    />
                  </button>
                  <div className="queue-actions">
                    <IconButton
                      icon="arrowDown_xs"
                      className="icon-button queue-move-up"
                      label={`Выше в очереди: ${t.title}`}
                      disabled={i === 0}
                      onClick={() => a.moveQueue(i, i - 1)}
                    />
                    <IconButton
                      icon="arrowDown_xs"
                      label={`Ниже в очереди: ${t.title}`}
                      disabled={i === a.queue.length - 1}
                      onClick={() => a.moveQueue(i, i + 1)}
                    />
                    <IconButton
                      icon="close_xs"
                      label={`Удалить из очереди: ${t.title}`}
                      disabled={i === a.index}
                      onClick={() => a.removeQueue(i)}
                    />
                  </div>
                </li>
              ))}
            </ol>
          ) : (
            <p className="muted">Выберите трек, чтобы начать слушать.</p>
          )}
        </>
      ) : (
        <div className="lyrics-empty">
          <Icon name="lyrics_xxs" size={60} />
          <h3>{a.current?.title || "Трек не выбран"}</h3>
          <p>Текст этого трека пока недоступен.</p>
        </div>
      )}
    </aside>
  );
}
