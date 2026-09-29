import { useEffect, useState } from "react";
import { useApp } from "../state/context.js";
import { post } from "../lib/api.js";
import { readStorage } from "../lib/library.js";
import catalog from "../data/catalog.json";
import Icon from "./Icon.jsx";

function sharpCover(url) {
  return String(url).replace(/-large\.(jpe?g|png|webp)$/i, "-t500x500.$1");
}

const covers = [
  ...new Set(
    catalog
      .map((track) => track.artwork)
      .filter(Boolean)
      .map(sharpCover),
  ),
];
const tiles = Array.from(
  { length: 48 },
  (_, index) => covers[index % covers.length],
);

export default function AuthModal() {
  const app = useApp();
  const [view, setView] = useState(
    app.authIntent === "switch" ? "switch" : "login",
  );
  const [more, setMore] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const remembered = readStorage("mixora-ui:accounts", []).filter(
    (account) => account?.id && account.id !== app.user?.id,
  );

  useEffect(() => {
    if (!app.user && view === "switch") setView("login");
  }, [app.user, view]);

  const close = () => app.setAuthOpen(false);
  const fail = (err) => setError(err.message || "Не удалось войти.");
  async function pickAccount(account) {
    setError("");
    setMore(false);
    if (!account?.token) {
      setEmail(account?.email || "");
      setPassword("");
      setView("password");
      return;
    }
    setBusy(true);
    try {
      await app.resume(account.token);
    } catch (err) {
      setEmail(account.email || "");
      setPassword("");
      setView("password");
      setError(
        err.status === 401
          ? "Сессия этого аккаунта закончилась. Введите пароль."
          : err.message || "Не удалось войти.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function submitLogin(event) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await app.login(email.trim(), password);
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  async function submitRegister(event) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await post("/auth/register", {
        email: email.trim(),
        password,
        display_name: name.trim(),
      });
      await app.login(email.trim(), password);
      app.toast("Аккаунт создан. Проверьте почту для подтверждения адреса.");
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  async function submitPasswordRequest(event) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await post("/auth/password/request", { email: email.trim() });
      setView("forgot-sent");
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      className="auth-wall"
      role="dialog"
      aria-modal="true"
      aria-label="Вход в Mixora"
    >
      <div className="auth-collage" aria-hidden="true">
        <div
          className={`auth-collage-move ${app.settings.animation ? "" : "paused"}`}
        >
          {[0, 1].map((copy) => (
            <div className="auth-collage-grid" key={copy}>
              {tiles.map((src, index) => (
                <img
                  key={`${copy}-${src}-${index}`}
                  src={src}
                  alt=""
                  width="500"
                  height="500"
                  decoding="async"
                />
              ))}
            </div>
          ))}
        </div>
      </div>
      <div className="auth-shade" />
      <section className={`auth-card ${view === "switch" ? "is-switch" : ""}`}>
        <header className="auth-card-bar">
          {view !== "login" || app.user ? (
            <button
              className="auth-icon"
              aria-label="Назад"
              onClick={() => {
                setError("");
                setMore(false);
                if (view === "login" && app.user) {
                  setView("switch");
                  return;
                }
                if (view === "switch") {
                  close();
                  return;
                }
                setView(app.user ? "switch" : "login");
              }}
            >
              <Icon name="arrowLeft_xs" size={22} />
            </button>
          ) : (
            <span />
          )}
          <button className="auth-icon" aria-label="Закрыть" onClick={close}>
            <Icon name="close_xs" size={20} />
          </button>
        </header>

        {view === "switch" && app.user ? (
          <SwitchAccount
            user={app.user}
            others={remembered}
            onPick={(account) => pickAccount(account)}
            busy={busy}
            onLogout={async () => {
              await app.logout();
              setView("login");
            }}
            onAdd={() => {
              setEmail("");
              setPassword("");
              setError("");
              setView("login");
            }}
          />
        ) : view === "register" ? (
          <form className="auth-form" onSubmit={submitRegister}>
            <h2>Создать аккаунт</h2>
            <p>Один экран для входа, регистрации и смены аккаунта Mixora.</p>
            <label>
              Как вас зовут
              <input
                value={name}
                onChange={(event) => setName(event.target.value)}
                autoComplete="nickname"
                required
                maxLength={80}
              />
            </label>
            <label>
              Электронная почта
              <input
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                autoComplete="email"
                required
              />
            </label>
            <label>
              Пароль
              <input
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="new-password"
                required
                minLength={12}
              />
            </label>
            <small>Пароль: не меньше 12 символов, до 72 байт.</small>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <button className="auth-submit" disabled={busy}>
              {busy ? "Подождите…" : "Создать аккаунт"}
            </button>
          </form>
        ) : view === "password" ? (
          <form className="auth-form" onSubmit={submitLogin}>
            <h2>Введите пароль</h2>
            <p>{email}</p>
            <label>
              Пароль
              <input
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
                autoFocus
                required
              />
            </label>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <button className="auth-submit" disabled={busy}>
              {busy ? "Подождите…" : "Войти"}
            </button>
            <button
              className="text-button"
              type="button"
              onClick={() => {
                setError("");
                setView("forgot");
              }}
            >
              Забыли пароль?
            </button>
          </form>
        ) : view === "forgot" ? (
          <form className="auth-form" onSubmit={submitPasswordRequest}>
            <h2>Восстановить пароль</h2>
            <p>Отправим ссылку для создания нового пароля.</p>
            <label>
              Электронная почта
              <input
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                autoComplete="email"
                autoFocus
                required
              />
            </label>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <button className="auth-submit" disabled={busy}>
              {busy ? "Отправляем…" : "Получить ссылку"}
            </button>
          </form>
        ) : view === "forgot-sent" ? (
          <div className="auth-form">
            <h2>Проверьте почту</h2>
            <p>
              Если аккаунт с адресом <strong>{email}</strong> существует, письмо
              со ссылкой уже отправлено.
            </p>
            <button
              className="auth-submit"
              type="button"
              onClick={() => setView("login")}
            >
              Вернуться ко входу
            </button>
          </div>
        ) : view === "qr" ? (
          <div className="auth-form">
            <h2>QR-код</h2>
            <p>
              Вход по QR-коду в Mixora пока не подключён. Войдите почтой на этом
              же экране.
            </p>
            <button
              className="auth-submit"
              type="button"
              onClick={() => setView("login")}
            >
              Войти почтой
            </button>
          </div>
        ) : (
          <form
            className="auth-form"
            onSubmit={(event) => {
              event.preventDefault();
              setError("");
              setView("password");
            }}
          >
            <div className="auth-brand">Mixora</div>
            <h2>Введите почту</h2>
            <p>Остался один шаг до Mixora</p>
            <label className="auth-identifier">
              <span className="auth-prefix" aria-hidden="true">
                @
              </span>
              <input
                type="email"
                value={email}
                placeholder="name@example.test"
                autoComplete="username"
                autoFocus
                required
                onChange={(event) => setEmail(event.target.value)}
              />
              {email && (
                <button
                  type="button"
                  className="auth-clear"
                  aria-label="Очистить почту"
                  onClick={() => setEmail("")}
                >
                  <Icon name="close_xxs" size={14} />
                </button>
              )}
            </label>
            {remembered.length > 0 && (
              <div className="auth-saved">
                {remembered.map((account) => (
                  <button
                    key={account.id}
                    type="button"
                    className="auth-other"
                    disabled={busy}
                    onClick={() => pickAccount(account)}
                  >
                    <span
                      className={`account-avatar ${account.plus ? "plus" : ""}`}
                    >
                      <span>
                        {account.display_name?.[0]?.toUpperCase() || "?"}
                      </span>
                    </span>
                    <span className="auth-person">
                      <strong>{account.display_name}</strong>
                      <small>{account.email}</small>
                    </span>
                  </button>
                ))}
              </div>
            )}
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <button className="auth-submit" disabled={busy}>
              Войти
            </button>
            <div className="auth-secondary">
              <button type="button" onClick={() => setView("qr")}>
                <Icon name="chain_xs" size={18} />
                QR-код
              </button>
              <button
                type="button"
                aria-expanded={more}
                onClick={() => setMore((value) => !value)}
              >
                Ещё
              </button>
            </div>
            {more && (
              <div className="auth-more" role="menu">
                <button
                  type="button"
                  role="menuitem"
                  onClick={() => {
                    setMore(false);
                    setView("register");
                  }}
                >
                  Создать аккаунт
                </button>
                <button
                  type="button"
                  role="menuitem"
                  onClick={() => {
                    setMore(false);
                    if (app.user) setView("switch");
                    else setError("Сначала войдите, чтобы сменить аккаунт.");
                  }}
                >
                  Сменить аккаунт
                </button>
              </div>
            )}
          </form>
        )}
      </section>
      <p className="auth-legal">© 2026 Mixora</p>
    </div>
  );
}

function Mark({ children }) {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" aria-hidden="true">
      {children}
    </svg>
  );
}

function SwitchAccount({ user, others, onPick, onLogout, onAdd, busy }) {
  return (
    <div className="auth-switch">
      <h2>Хотите войти в другой аккаунт?</h2>
      <div className="auth-current">
        <span className={`account-avatar ${user.plus ? "plus" : ""}`}>
          <span>{user.display_name?.[0]?.toUpperCase() || "?"}</span>
          <i className="auth-check" aria-hidden="true" />
        </span>
        <span className="auth-person">
          <strong>{user.display_name}</strong>
          <small>{user.email}</small>
        </span>
      </div>
      {others.map((account) => (
        <button
          key={account.id}
          className="auth-other"
          disabled={busy}
          onClick={() => onPick(account)}
        >
          <span className={`account-avatar ${account.plus ? "plus" : ""}`}>
            <span>{account.display_name?.[0]?.toUpperCase() || "?"}</span>
          </span>
          <span className="auth-person">
            <strong>{account.display_name}</strong>
            <small>{account.email}</small>
          </span>
        </button>
      ))}
      <button className="auth-row" onClick={onLogout}>
        <Mark>
          <path
            d="M10 6V5a1 1 0 0 1 1-1h8v16h-8a1 1 0 0 1-1-1v-1"
            stroke="currentColor"
            strokeWidth="1.6"
          />
          <path
            d="M4 12h10M11 8l4 4-4 4"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </Mark>
        Выйти
      </button>
      <button className="auth-row" onClick={onAdd}>
        <Mark>
          <circle cx="9" cy="8" r="3" stroke="currentColor" strokeWidth="1.6" />
          <path
            d="M3.5 19c.8-3 2.7-4.5 5.5-4.5 1 0 1.9.2 2.7.6"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
          />
          <path
            d="M17 11v6M14 14h6"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
          />
        </Mark>
        Добавить аккаунт
      </button>
    </div>
  );
}
