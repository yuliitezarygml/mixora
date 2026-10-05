import { useEffect } from "react";
import { useApp } from "../state/context.js";
import { duration, trackKey } from "../lib/library.js";
import Icon from "./Icon.jsx";
import { Cover } from "./Primitives.jsx";

function QueueRow({ track, current, onClick }) {
  return (
    <button
      className={`HorizontalCardContainer_root__YoAAP${current ? " is-current" : ""}`}
      onClick={onClick}
    >
      <Cover track={track} />
      <span className="EntityMeta_root__Zn4Th">
        <strong className="EntityMeta_title__6_ChR">{track.title}</strong>
        <small className="EntityMeta_subtitle__yE1NK">{track.artist}</small>
      </span>
      <em className="queue-time">{duration(track.duration)}</em>
    </button>
  );
}

export default function FullscreenPlayer() {
  const app = useApp();
  const track = app.current;
  const upcoming = app.queue.slice(app.index + 1, app.index + 8);
  const progress = app.length
    ? Math.min(100, (app.position / app.length) * 100)
    : 0;
  useEffect(() => {
    const onKey = (event) => {
      if (event.key === "Escape") app.setPanel(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [app]);
  return (
    <section
      className="FullscreenPlayerDesktop_root___8vo1 FullscreenPlayerDesktop_important__dGfiL"
      aria-label="Полный экран плеера"
    >
      <header className="FullscreenPlayerDesktop_header__OBhzq">
        <button
          className="FullscreenPlayerDesktop_closeButton__MQ64s"
          aria-label="Свернуть плеер"
          onClick={() => app.setPanel(null)}
        >
          <Icon name="arrowDown_xs" size={24} />
        </button>
      </header>
      <div className="FullscreenPlayerDesktop_modalContent__Zs_LC">
        <div className="FullscreenPlayerDesktopContent_root__tKNGK">
          <div className="FullscreenPlayerDesktopContent_fullscreenContent__Nvety">
            <div className="FullscreenPlayerDesktopPoster_root__d__YD">
              {track?.artwork ? (
                <img
                  className="FullscreenPlayerDesktopPoster_cover__CDmhM"
                  src={track.artwork.replace("-large.", "-t500x500.")}
                  alt=""
                />
              ) : (
                <div className="FullscreenPlayerDesktopPoster_cover__CDmhM" />
              )}
            </div>
            <div className="FullscreenPlayerDesktopContent_info__Dq69p">
              <div className="FullscreenPlayerDesktopContent_meta__3jDTy FullscreenPlayerDesktopContent_meta_isSplitMode__zPC2S">
                <div className="FullscreenPlayerDesktopContent_title__I2JrP">
                  {track?.title || "Выберите трек"}
                </div>
                <div className="FullscreenPlayerDesktopContent_artists__a_2G3">
                  {track?.artist}
                </div>
              </div>
              <div className="FullscreenPlayerDesktopContent_sliderContainer__FtBZ7">
                <label className="FullscreenPlayerDesktopContent_slider__FJscl">
                  <input
                    aria-label="Позиция воспроизведения"
                    type="range"
                    min="0"
                    max={app.length || 1}
                    step="0.1"
                    value={Math.min(app.position, app.length || 1)}
                    disabled={!app.length}
                    style={{ "--progress": `${progress}%` }}
                    onChange={(event) => app.seek(Number(event.target.value))}
                  />
                </label>
              </div>
            </div>
          </div>
          <div className="FullscreenPlayerDesktopContent_additionalContent__tuuy7">
            <div className="PlayQueue_root__ponhw">
              <div className="PlayQueue_content__zIUvd">
                <div className="PlayQueue_scrollContent__2dI0v">
                  <div className="PlayQueueTitle_root__E2XOW">
                    <div className="PlayQueueTitle_modeTitle__KixWV">
                      Сейчас играет
                    </div>
                    <h2 className="PlayQueueTitle_heading__JrzQq">
                      {app.waveActive ? "Моя волна" : "Очередь"}
                    </h2>
                  </div>
                  <div className="PlayQueueNowPlayingBlock_root__aJSb8">
                    {track && (
                      <QueueRow track={track} current onClick={app.toggle} />
                    )}
                  </div>
                  <div className="PlayQueueAfterPlayingBlock_root__A7_wI">
                    <div className="PlayQueueAfterPlayingBlock_title__nS_nG">
                      Далее в очереди
                    </div>
                    {upcoming.map((item, offset) => (
                      <QueueRow
                        key={`${trackKey(item)}-${offset}`}
                        track={item}
                        onClick={() => app.setIndex(app.index + 1 + offset)}
                      />
                    ))}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
