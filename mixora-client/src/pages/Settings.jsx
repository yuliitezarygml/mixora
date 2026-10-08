import { useState } from "react";
import { Link } from "react-router-dom";
import { useApp } from "../state/context.js";
import { Modal } from "../components/Primitives.jsx";
export default function Settings() {
  const a = useApp();
  const [clear, setClear] = useState(false);
  return (
    <div className="page-padding settings-page SettingsPage_content__cR6Ra">
      <h1>Настройки</h1>
      <section className="settings-section Settings_root__FVVrn">
        <h2>Аккаунт</h2>
        <div className="setting-row">
          <div>
            <strong>{a.user?.display_name || "Войдите в Mixora"}</strong>
            <p>{a.user?.email || "Слушайте музыку через свой аккаунт"}</p>
          </div>
          <button
            className="secondary"
            onClick={() => (a.user ? a.logout() : a.setAuthOpen(true))}
          >
            {a.user ? "Выйти" : "Войти"}
          </button>
        </div>
      </section>
      <section className="settings-section Settings_root__FVVrn">
        <h2>Музыкальные интересы</h2>
        <div className="setting-row">
          <div>
            <strong>Музыкальные интересы</strong>
            <p>Артисты и жанры для Моей волны</p>
          </div>
          <button
            className="secondary"
            onClick={() =>
              a.user ? a.setTasteOpen(true) : a.setAuthOpen(true)
            }
          >
            Уточнить предпочтения
          </button>
        </div>
      </section>
      <section className="settings-section Settings_root__FVVrn">
        <h2>Внешний вид</h2>
        <div className="setting-row">
          <label htmlFor="theme">Тема приложения</label>
          <select
            id="theme"
            value={a.settings.theme}
            onChange={(e) => a.setSettings({ theme: e.target.value })}
          >
            <option value="dark">Тёмная</option>
            <option value="light">Светлая</option>
          </select>
        </div>
        <Toggle
          title="Анимация Моей волны"
          text="Движение на главной странице"
          value={a.settings.animation}
          onChange={(v) => a.setSettings({ animation: v })}
        />
      </section>
      <section className="settings-section Settings_root__FVVrn">
        <h2>Воспроизведение</h2>
        <Toggle
          title="Продолжать воспроизведение"
          text="Переходить к следующему треку в очереди"
          value={a.settings.autoplay}
          onChange={(v) => a.setSettings({ autoplay: v })}
        />
        <Toggle
          title="Оригинальные версии треков"
          text="Показывать музыку с отметкой Explicit"
          value={a.settings.explicit}
          onChange={(v) => a.setSettings({ explicit: v })}
        />
        <div className="setting-row">
          <span>Громкость</span>
          <input
            aria-label="Основная громкость"
            type="range"
            min="0"
            max="1"
            step=".01"
            value={a.settings.volume}
            onChange={(e) => a.setSettings({ volume: Number(e.target.value) })}
          />
        </div>
      </section>
      <section className="settings-section Settings_root__FVVrn">
        <h2>Библиотека</h2>
        <div className="setting-row">
          <div>
            <strong>История прослушивания</strong>
            <p>Синхронизируется с вашим аккаунтом Mixora</p>
          </div>
          <button className="secondary" onClick={() => setClear(true)}>
            Очистить
          </button>
        </div>
        <Link className="setting-row" to="/collection/dislikes">
          Треки, которые не нравятся<span>Открыть →</span>
        </Link>
      </section>
      <section className="settings-section Settings_root__FVVrn">
        <h2>О приложении</h2>
        <p className="muted">Mixora · 0.1.0</p>
        <p>Музыка и ваши плейлисты в одном месте.</p>
        <p className="muted">
          Сохранённая коллекция синхронизируется с вашим аккаунтом Mixora.
          Источники: SoundCloud, YouTube, VK, Bandcamp и доступные Spotify
          preview.
        </p>
      </section>
      {clear && (
        <Modal title="Очистить историю?" onClose={() => setClear(false)}>
          <p>История прослушивания будет удалена из вашего аккаунта Mixora.</p>
          <button
            className="danger"
            onClick={() => {
              void a.clearHistory().then((cleared) => {
                if (cleared) setClear(false);
              });
            }}
          >
            Очистить
          </button>
        </Modal>
      )}
    </div>
  );
}
function Toggle({ title, text, value, onChange }) {
  return (
    <label className="setting-row SettingsListToggleItem_root__yEEYT">
      <div>
        <strong>{title}</strong>
        <p>{text}</p>
      </div>
      <input
        className="switch"
        type="checkbox"
        checked={value}
        onChange={(e) => onChange(e.target.checked)}
      />
    </label>
  );
}
