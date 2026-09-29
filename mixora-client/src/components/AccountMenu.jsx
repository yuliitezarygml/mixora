import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useNavigate } from "react-router-dom";
import { useApp } from "../state/context.js";
import Icon from "./Icon.jsx";

export default function AccountMenu({ collapsed }) {
  const app = useApp();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [panel, setPanel] = useState("main");
  const [place, setPlace] = useState(null);
  const root = useRef(null);
  const menu = useRef(null);
  const button = useRef(null);
  const plus = app.user?.plus === true;
  const name = app.user?.display_name || "Войти";
  const handle = app.user?.email?.split("@")[0] || "";

  useEffect(() => {
    if (!open) return;
    const placeMenu = () => {
      const rect = button.current?.getBoundingClientRect();
      if (!rect) return;
      setPlace({
        left: Math.max(8, rect.left),
        bottom: Math.max(8, window.innerHeight - rect.top + 10),
      });
    };
    const close = (event) => {
      if (
        root.current?.contains(event.target) ||
        menu.current?.contains(event.target)
      ) {
        return;
      }
      setOpen(false);
    };
    const onKey = (event) => {
      if (event.key === "Escape") setOpen(false);
    };
    placeMenu();
    document.addEventListener("pointerdown", close);
    document.addEventListener("keydown", onKey);
    window.addEventListener("resize", placeMenu);
    return () => {
      document.removeEventListener("pointerdown", close);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("resize", placeMenu);
    };
  }, [open]);

  const openMenu = () => {
    if (!app.user) {
      app.setAuthOpen(true);
      return;
    }
    setPanel("main");
    setOpen((value) => !value);
  };

  return (
    <div
      className="NavbarDesktopUserWidget_userProfileContainer__ha3Tm sidebar-account"
      ref={root}
    >
      <button
        ref={button}
        className="account-button"
        aria-label="Меню аккаунта"
        aria-expanded={open}
        onClick={openMenu}
      >
        <Avatar name={name} plus={plus} />
        {!collapsed && (
          <span className="account-caption">
            <span className="account-name">{name}</span>
            {plus && (
              <span className="plus-pill">
                <Icon name="plusColor" size={16} />
                Плюс
              </span>
            )}
          </span>
        )}
      </button>
      {open &&
        app.user &&
        createPortal(
          <div
            ref={menu}
            className="account-menu"
            role="dialog"
            aria-label="Аккаунт"
            style={place || undefined}
          >
            <button
              className="account-close"
              aria-label="Закрыть меню аккаунта"
              onClick={() => setOpen(false)}
            >
              <Icon name="close_xs" size={18} />
            </button>
            {panel === "appearance" ? (
              <Appearance
                theme={app.settings.theme}
                onTheme={(theme) => app.setSettings({ theme })}
                onBack={() => setPanel("main")}
              />
            ) : (
              <>
                <div className="account-hero">
                  <Avatar name={name} plus={plus} large />
                  <strong>{name}</strong>
                  <p>
                    {app.user.email}
                    {handle ? ` · ${handle}` : ""}
                  </p>
                </div>
                <button
                  className="plus-card"
                  onClick={() => app.setPlus(!plus)}
                >
                  <Icon name="plusColor" size={28} />
                  <span className="plus-copy">
                    <strong>Плюс</strong>
                    <small>
                      {plus ? "Подписка активна" : "Подключить подписку Mixora"}
                    </small>
                  </span>
                  <span className={`plus-badge ${plus ? "on" : ""}`}>
                    <Icon name="plusColor" size={16} />
                  </span>
                </button>
                <nav className="account-links">
                  <button
                    onClick={() => {
                      setOpen(false);
                      navigate("/settings");
                    }}
                  >
                    <Glyph>
                      <rect
                        x="4"
                        y="4"
                        width="16"
                        height="16"
                        rx="4"
                        stroke="currentColor"
                        strokeWidth="1.6"
                      />
                      <text
                        x="12"
                        y="15.2"
                        textAnchor="middle"
                        fontSize="7"
                        fontWeight="700"
                        fill="currentColor"
                      >
                        ID
                      </text>
                    </Glyph>
                    Управление аккаунтом
                  </button>
                  <button
                    onClick={() => {
                      setOpen(false);
                      navigate("/settings");
                    }}
                  >
                    <Icon name="settings_xs" size={22} />
                    Настройки
                  </button>
                  <button
                    onClick={() =>
                      app.toast("Чат с поддержкой Mixora пока не подключён.")
                    }
                  >
                    <Glyph>
                      <path
                        d="M4 12.5 20 4 13.5 20l-2.2-6.2L4 12.5Z"
                        stroke="currentColor"
                        strokeWidth="1.6"
                        strokeLinejoin="round"
                      />
                    </Glyph>
                    Чат с поддержкой
                  </button>
                  <button
                    onClick={() =>
                      app.toast("Интерфейс Mixora сейчас на русском языке.")
                    }
                  >
                    <Glyph>
                      <circle
                        cx="12"
                        cy="12"
                        r="8"
                        stroke="currentColor"
                        strokeWidth="1.6"
                      />
                      <path
                        d="M4 12h16M12 4c2.2 2.4 3.3 5.1 3.3 8S14.2 17.6 12 20c-2.2-2.4-3.3-5.1-3.3-8S9.8 6.4 12 4Z"
                        stroke="currentColor"
                        strokeWidth="1.6"
                      />
                    </Glyph>
                    Русский
                  </button>
                  <hr />
                  <button onClick={() => setPanel("appearance")}>
                    <Glyph>
                      <rect
                        x="3"
                        y="5"
                        width="18"
                        height="12"
                        rx="2"
                        stroke="currentColor"
                        strokeWidth="1.6"
                      />
                      <path
                        d="M9 20h6"
                        stroke="currentColor"
                        strokeWidth="1.6"
                        strokeLinecap="round"
                      />
                    </Glyph>
                    Внешний вид
                  </button>
                  <hr />
                  <button
                    onClick={() => {
                      setOpen(false);
                      app.setAuthIntent("switch");
                      app.setAuthOpen(true);
                    }}
                  >
                    <Glyph>
                      <circle
                        cx="10"
                        cy="8"
                        r="3"
                        stroke="currentColor"
                        strokeWidth="1.6"
                      />
                      <path
                        d="M4.5 19c.8-3 2.8-4.5 5.5-4.5 1.2 0 2.3.3 3.2.8M16 14l3 3-3 3M19 17h-6"
                        stroke="currentColor"
                        strokeWidth="1.6"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      />
                    </Glyph>
                    Сменить аккаунт
                  </button>
                </nav>
              </>
            )}
          </div>,
          document.body,
        )}
    </div>
  );
}

function Avatar({ name, plus, large = false }) {
  return (
    <span
      className={`account-avatar ${plus ? "plus" : ""} ${large ? "large" : ""}`}
    >
      <span>{name?.[0]?.toUpperCase() || "?"}</span>
    </span>
  );
}

function Appearance({ theme, onTheme, onBack }) {
  return (
    <div className="account-appearance">
      <button className="account-back" onClick={onBack}>
        Назад
      </button>
      <h2>Внешний вид</h2>
      {[
        ["dark", "Тёмная"],
        ["light", "Светлая"],
      ].map(([value, label]) => (
        <button
          key={value}
          className={theme === value ? "selected" : ""}
          aria-pressed={theme === value}
          onClick={() => onTheme(value)}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

function Glyph({ children }) {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" aria-hidden="true">
      {children}
    </svg>
  );
}
