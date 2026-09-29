import { useState } from "react";
import { useApp } from "../state/context.js";
import {
  defaultWave,
  normalizeWave,
  waveActivities,
  waveCharacters,
  waveLanguages,
  waveMoods,
} from "../lib/wave.js";
import Icon from "./Icon.jsx";

export default function WaveSettings() {
  const app = useApp();
  const [preferences, setPreferences] = useState(() =>
    normalizeWave(app.wavePreferences),
  );
  const apply = (next) => {
    const wave = normalizeWave(next);
    setPreferences(wave);
    app.setSettings({ wave });
    if (app.waveActive) app.startWave(app.waveContext, wave);
  };
  const toggle = (key, value) =>
    apply({
      ...preferences,
      [key]: preferences[key] === value ? "any" : value,
    });

  return (
    <div className="wave-settings-layer">
      <button
        className="wave-settings-backdrop"
        aria-label="Закрыть настройки волны"
        onClick={() => app.setWaveSettingsOpen(false)}
      />
      <section
        className="VibeSettings_popover__VKqUc wave-settings-card"
        aria-label="Настроить Мою волну"
      >
        <header className="VibeSettings_header__OAWe2">
          <h2>Настроить Мою волну</h2>
          <div className="VibeSettings_actions__ckbMt">
            <button
              className="wave-settings-icon wave-settings-icon--reset"
              aria-label="Сбросить настройки волны"
              onClick={() => apply(defaultWave)}
            >
              <Icon name="reset_xxs" size={18} />
            </button>
            <button
              className="wave-settings-icon wave-settings-icon--close"
              aria-label="Закрыть"
              onClick={() => app.setWaveSettingsOpen(false)}
            >
              <Icon name="close_xs" size={16} />
            </button>
          </div>
        </header>
        <div className="VibeRestrictions_root__efJez VibeSettings_root__ufZlV">
          <fieldset className="RestrictionBlock_root__P_g9o">
            <legend>Под занятие</legend>
            <div className="RestrictionBlock_restrictions__BhR_r VibeRestrictions_contextItems__JrF7D">
              {waveActivities.map(([value, label]) => (
                <Choice
                  key={value}
                  className={`wave-activity wave-activity--${value}`}
                  pressed={preferences.activity === value}
                  onClick={() => toggle("activity", value)}
                >
                  {label}
                </Choice>
              ))}
            </div>
          </fieldset>
          <fieldset className="RestrictionBlock_root__P_g9o">
            <legend>По характеру</legend>
            <div className="RestrictionBlock_restrictions__BhR_r VibeRestrictions_diversity__qfOls">
              {waveCharacters.map(([value, label]) => (
                <button
                  key={value}
                  aria-pressed={preferences.diversity === value}
                  className={`RestrictionButton_button__eLMkU RestrictionButton_diversityButton__uclSi ${preferences.diversity === value ? "RestrictionButton_button_selected__LHD20" : ""}`}
                  onClick={() => toggle("diversity", value)}
                >
                  <span className="RestrictionButton_diversityButtonImage__21oME">
                    <CharacterIcon name={value} />
                  </span>
                  <span className="RestrictionButton_title__UZn0O">
                    {label}
                  </span>
                </button>
              ))}
            </div>
          </fieldset>
          <fieldset className="RestrictionBlock_root__P_g9o">
            <legend>По настроению</legend>
            <div className="VibeRestrictions_moodEnergy__Le0Cy">
              {waveMoods.map(([value, label, color]) => (
                <button
                  key={value}
                  aria-pressed={preferences.mood === value}
                  className={`RestrictionButton_moodEnergy__q_I4y ${preferences.mood === value ? "is-selected" : ""}`}
                  onClick={() => toggle("mood", value)}
                >
                  <span className="RestrictionButton_moodEnergyButton__yKkaS wave-mood-button">
                    <span
                      className={`wave-mood wave-mood--${value}`}
                      style={{ "--wave-mood-fallback": color }}
                    />
                  </span>
                  <span className="RestrictionButton_title__UZn0O">
                    {label}
                  </span>
                </button>
              ))}
            </div>
          </fieldset>
          <fieldset className="RestrictionBlock_root__P_g9o">
            <legend>По языку</legend>
            <div className="RestrictionBlock_restrictions__BhR_r">
              {waveLanguages.map(([value, label]) => (
                <Choice
                  key={value}
                  pressed={preferences.language === value}
                  onClick={() => toggle("language", value)}
                >
                  {label}
                </Choice>
              ))}
            </div>
          </fieldset>
        </div>
      </section>
    </div>
  );
}

function CharacterIcon({ name }) {
  if (name === "favorite") {
    return (
      <svg
        className="wave-character-icon"
        viewBox="0 0 48 48"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id="wave-heart-fill" x1="8" y1="4" x2="35" y2="44">
            <stop offset="0" stopColor="#ff5148" />
            <stop offset="0.52" stopColor="#ed172b" />
            <stop offset="1" stopColor="#a90019" />
          </linearGradient>
          <radialGradient
            id="wave-heart-shine"
            cx="0"
            cy="0"
            r="1"
            gradientTransform="translate(15 10) rotate(48) scale(19 16)"
          >
            <stop stopColor="#ffb7a8" stopOpacity="0.95" />
            <stop offset="0.45" stopColor="#ff5448" stopOpacity="0.45" />
            <stop offset="1" stopColor="#ff2837" stopOpacity="0" />
          </radialGradient>
        </defs>
        <path
          d="M24 42.5C20.8 37.7 6 29.8 6 17.4 6 10.9 10.7 6.5 16.7 6.5c3.8 0 6.6 2 8.2 5 1.8-3 4.7-5 8.4-5 6 0 10.2 4.6 9.7 11C42.1 29.3 28.1 38.2 24 42.5Z"
          fill="url(#wave-heart-fill)"
        />
        <path
          d="M24.9 11.4C22.9 18.2 24.8 25.2 22.7 34.7l2.1 7.4c1.8-6.5.2-13.2 2.1-20.7 1.4-5.5.2-8.2-2-10Z"
          fill="#5f0012"
          opacity="0.7"
        />
        <path
          d="M24 42.5C20.8 37.7 6 29.8 6 17.4 6 10.9 10.7 6.5 16.7 6.5c3.8 0 6.6 2 8.2 5 1.8-3 4.7-5 8.4-5 6 0 10.2 4.6 9.7 11C42.1 29.3 28.1 38.2 24 42.5Z"
          fill="url(#wave-heart-shine)"
        />
      </svg>
    );
  }

  if (name === "unknown") {
    return (
      <svg
        className="wave-character-icon"
        viewBox="0 0 48 48"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id="wave-spark-fill" x1="9" y1="6" x2="38" y2="42">
            <stop offset="0" stopColor="#fffaa2" />
            <stop offset="0.46" stopColor="#ffd31a" />
            <stop offset="1" stopColor="#f17d00" />
          </linearGradient>
          <radialGradient
            id="wave-spark-glow"
            cx="0"
            cy="0"
            r="1"
            gradientTransform="translate(25 23) scale(12)"
          >
            <stop stopColor="#fffbd0" />
            <stop offset="0.4" stopColor="#ffe227" stopOpacity="0.8" />
            <stop offset="1" stopColor="#ff9800" stopOpacity="0" />
          </radialGradient>
        </defs>
        <path
          d="M24.3 2.8c2.1 12.6 6.6 17.3 20.4 20.2-13.6 2.8-18 7.5-20.4 22.1C22 30.8 17.7 26.1 3.3 23 17.7 20.2 22 15.5 24.3 2.8Z"
          fill="url(#wave-spark-fill)"
        />
        <path
          d="M24.3 2.8c.7 13.4 1.8 17.4 20.4 20.2-16.9.1-19.4 3.1-20.4 22.1-1.3-17.2-4.5-20.3-21-22.1 17.8-1.4 20.3-5.4 21-20.2Z"
          fill="url(#wave-spark-glow)"
          opacity="0.85"
        />
      </svg>
    );
  }

  return (
    <svg className="wave-character-icon" viewBox="0 0 48 48" aria-hidden="true">
      <defs>
        <linearGradient id="wave-lightning-fill" x1="12" y1="5" x2="34" y2="43">
          <stop offset="0" stopColor="#baff7a" />
          <stop offset="0.45" stopColor="#32ee75" />
          <stop offset="1" stopColor="#00a94f" />
        </linearGradient>
        <linearGradient
          id="wave-lightning-shine"
          x1="17"
          y1="8"
          x2="27"
          y2="25"
        >
          <stop stopColor="#efffc3" stopOpacity="0.9" />
          <stop offset="1" stopColor="#84ff8b" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path
        d="m28.7 3.6-20 20.7 12.4-.5-7.2 20.7 26-25.7-13.5 2.1 8.5-17.3H28.7Z"
        fill="url(#wave-lightning-fill)"
      />
      <path
        d="M28.7 3.6 8.7 24.3l12.4-.5L34.9 3.6h-6.2Z"
        fill="url(#wave-lightning-shine)"
      />
    </svg>
  );
}

function Choice({ pressed, onClick, children, className = "" }) {
  return (
    <button
      aria-pressed={pressed}
      className={`RestrictionButton_button__eLMkU RestrictionButton_textButton__HC_AE ${className} ${pressed ? "RestrictionButton_button_selected__LHD20" : ""}`}
      onClick={onClick}
    >
      {children}
    </button>
  );
}
