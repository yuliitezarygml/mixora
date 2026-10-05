import { useApp } from "../state/context.js";
import { Modal } from "./Primitives.jsx";

const qualities = [
  [
    "high",
    "Превосходное",
    "Максимальное качество потока, если источник его отдаёт.",
  ],
  [
    "optimal",
    "Оптимальное",
    "Сбалансированный звук для большинства устройств.",
  ],
  ["economy", "Экономичное", "Стабильное звучание при медленном интернете."],
];

export default function SoundSettings({ onClose }) {
  const app = useApp();
  const quality = app.settings.quality || "optimal";
  return (
    <Modal title="Настройки звука" onClose={onClose}>
      <div className="sound-settings">
        {qualities.map(([value, title, text]) => (
          <button
            key={value}
            className={quality === value ? "selected" : ""}
            aria-pressed={quality === value}
            onClick={() => app.setSettings({ quality: value })}
          >
            <span>
              <strong>{title}</strong>
              <small>{text}</small>
            </span>
            {quality === value && <i aria-hidden="true" />}
          </button>
        ))}
        <label className="sound-eq">
          <span>Эквалайзер</span>
          <input
            type="checkbox"
            className="switch"
            checked={app.settings.equalizer === true}
            onChange={(event) =>
              app.setSettings({ equalizer: event.target.checked })
            }
          />
        </label>
      </div>
    </Modal>
  );
}
