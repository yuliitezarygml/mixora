import { useEffect, useState } from "react";
import { useApp } from "../state/context.js";
import { duration, trackKey } from "../lib/library.js";
import Icon from "./Icon.jsx";
import { Cover, IconButton } from "./Primitives.jsx";
import FullscreenPlayer from "./FullscreenPlayer.jsx";
export default function Player() {
  const a = useApp();
  const [tint, setTint] = useState("");
  useEffect(() => {
    const artwork = a.current?.artwork;
    if (!artwork) {
      setTint("");
      document.documentElement.style.removeProperty("--player-tint");
      document.documentElement.style.removeProperty(
        "--player-average-color-background",
      );
      return;
    }
    let cancelled = false;
    const image = new Image();
    image.crossOrigin = "anonymous";
    image.onload = () => {
      const canvas = document.createElement("canvas");
      canvas.width = 12;
      canvas.height = 12;
      const context = canvas.getContext("2d", { willReadFrequently: true });
      context.drawImage(image, 0, 0, 12, 12);
      const pixels = context.getImageData(0, 0, 12, 12).data;
      let red = 0;
      let green = 0;
      let blue = 0;
      for (let index = 0; index < pixels.length; index += 4) {
        red += pixels[index];
        green += pixels[index + 1];
        blue += pixels[index + 2];
      }
      const count = pixels.length / 4;
      const tone = (channel) => Math.round((channel / count) * 0.48);
      if (!cancelled) {
        const color = `rgb(${tone(red)}, ${tone(green)}, ${tone(blue)})`;
        setTint(color);
        document.documentElement.style.setProperty("--player-tint", color);
        document.documentElement.style.setProperty(
          "--player-average-color-background",
          color,
        );
      }
    };
    image.onerror = () => {
      if (!cancelled) {
        setTint("");
        document.documentElement.style.removeProperty("--player-tint");
        document.documentElement.style.removeProperty(
          "--player-average-color-background",
        );
      }
    };
    image.src = artwork;
    return () => {
      cancelled = true;
    };
  }, [a.current?.artwork]);
  const liked =
    a.current &&
    a.library.likes.some((t) => trackKey(t) === trackKey(a.current));
  const progress = a.length
    ? Math.min(100, (a.position / a.length) * 100)
    : 0;
  return (
    <footer
      className={`player CommonLayout_playerBar__zXRxq PlayerBarDesktop_root__d2Hwi ${tint ? "player-live" : ""}`}
      aria-label="Музыкальный плеер"
      style={tint ? { "--player-tint": tint } : undefined}
    >
      <div className="player-notch" style={{ "--progress": `${progress}%` }}>
        <span>{duration(a.position)}</span>
        <input
          aria-label="Позиция воспроизведения"
          type="range"
          min="0"
          max={a.length || 1}
          value={Math.min(a.position, a.length || 1)}
          step=".1"
          disabled={!a.length}
          onChange={(e) => a.seek(Number(e.target.value))}
        />
        <span>{duration(a.length || a.current?.duration)}</span>
      </div>
      <div className="player-track PlayerBarDesktop_info__56v53">
        <Cover track={a.current} />
        <div className="player-track-text">
          <strong>{a.current?.title || "Выберите музыку"}</strong>
          {a.current ? (
            <span>{a.current.artist}</span>
          ) : (
            <span>Треки, которые хочется слушать</span>
          )}
        </div>
        {a.current && (
          <IconButton
            icon="more_xxs"
            label="Действия с треком"
          />
        )}
      </div>
      <div className="player-center PlayerBarDesktop_sonata__sJHY_">
        <div className="player-buttons BaseSonataControlsDesktop_root__E6wjA SonataControls_root__w8uqu">
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
              disabled={!a.catalog.length}
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
          icon="lyrics_xxs"
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
          {a.playbackError}
        </div>
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
                  <button onClick={() => a.setIndex(i)}>
                    <Cover track={t} />
                    <span>
                      <strong>{t.title}</strong>
                      <small>{t.artist}</small>
                    </span>
                    <Icon
                      name={a.index === i && a.playing ? "pause_xs" : "play_xs"}
                    />
                  </button>
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
