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
              className="wave-settings-icon"
              aria-label="Сбросить настройки волны"
              onClick={() => apply(defaultWave)}
            >
              <Icon name="reset_xxs" size={20} />
            </button>
            <button
              className="wave-settings-icon"
              aria-label="Закрыть"
              onClick={() => app.setWaveSettingsOpen(false)}
            >
              <Icon name="close_xs" size={20} />
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
              {waveCharacters.map(([value, label, icon]) => (
                <button
                  key={value}
                  aria-pressed={preferences.diversity === value}
                  className={`RestrictionButton_button__eLMkU RestrictionButton_diversityButton__uclSi ${preferences.diversity === value ? "RestrictionButton_button_selected__LHD20" : ""}`}
                  onClick={() => toggle("diversity", value)}
                >
                  <span className="RestrictionButton_diversityButtonImage__21oME">
                    <Icon name={icon} size={28} />
                  </span>
                  <span className="RestrictionButton_title__UZn0O">{label}</span>
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
                  <span
                    className="RestrictionButton_moodEnergyButton__yKkaS wave-mood"
                    style={{ background: color }}
                  />
                  <span className="RestrictionButton_title__UZn0O">{label}</span>
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

function Choice({ pressed, onClick, children }) {
  return (
    <button
      aria-pressed={pressed}
      className={`RestrictionButton_button__eLMkU RestrictionButton_textButton__HC_AE ${pressed ? "RestrictionButton_button_selected__LHD20" : ""}`}
      onClick={onClick}
    >
      {children}
    </button>
  );
}
