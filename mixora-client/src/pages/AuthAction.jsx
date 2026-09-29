import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { post } from "../lib/api.js";
import { useApp } from "../state/context.js";
import Icon from "../components/Icon.jsx";

export function authActionToken(search) {
  return new URLSearchParams(search).get("token")?.trim() || "";
}

export default function AuthAction({ action }) {
  const app = useApp();
  const location = useLocation();
  const navigate = useNavigate();
  const token = authActionToken(location.search);
  const [state, setState] = useState(action === "verify" ? "working" : "form");
  const [message, setMessage] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (action !== "verify") return;
    if (!token) {
      setState("error");
      setMessage("В ссылке нет кода подтверждения.");
      return;
    }
    let active = true;
    post("/auth/verify-email", { token })
      .then(() => {
        if (!active) return;
        setState("success");
        setMessage("Почта подтверждена. Аккаунт Mixora готов к работе.");
      })
      .catch((error) => {
        if (!active) return;
        setState("error");
        setMessage(error.message || "Не удалось подтвердить почту.");
      });
    return () => {
      active = false;
    };
  }, [action, token]);

  async function resetPassword(event) {
    event.preventDefault();
    setMessage("");
    if (!token) {
      setMessage("В ссылке нет кода сброса пароля.");
      return;
    }
    if (password !== confirmation) {
      setMessage("Пароли не совпадают.");
      return;
    }
    setBusy(true);
    try {
      await post("/auth/password/reset", { token, password });
      setState("success");
      setMessage("Новый пароль сохранён. Теперь можно войти в Mixora.");
    } catch (error) {
      setMessage(error.message || "Не удалось изменить пароль.");
    } finally {
      setBusy(false);
    }
  }

  const openLogin = () => {
    app.setAuthIntent("login");
    app.setAuthOpen(true);
    navigate("/", { replace: true });
  };

  return (
    <section className="auth-action-page">
      <div className="auth-action-card">
        <Icon name="musicLogo" size={54} />
        {action === "verify" ? (
          <>
            <h1>Подтверждение почты</h1>
            <p role={state === "error" ? "alert" : "status"}>
              {state === "working"
                ? "Проверяем ссылку…"
                : message || "Ссылка обработана."}
            </p>
          </>
        ) : state === "success" ? (
          <>
            <h1>Пароль изменён</h1>
            <p role="status">{message}</p>
          </>
        ) : (
          <form className="auth-form" onSubmit={resetPassword}>
            <h1>Новый пароль</h1>
            <p>Введите новый пароль для аккаунта Mixora.</p>
            <label>
              Новый пароль
              <input
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="new-password"
                required
                minLength={12}
              />
            </label>
            <label>
              Повторите пароль
              <input
                type="password"
                value={confirmation}
                onChange={(event) => setConfirmation(event.target.value)}
                autoComplete="new-password"
                required
                minLength={12}
              />
            </label>
            <small>Пароль: не меньше 12 символов, до 72 байт.</small>
            {message && (
              <p className="form-error" role="alert">
                {message}
              </p>
            )}
            <button className="auth-submit" disabled={busy || !token}>
              {busy ? "Сохраняем…" : "Сохранить пароль"}
            </button>
          </form>
        )}
        {(state === "success" || state === "error") && (
          <button className="auth-submit" type="button" onClick={openLogin}>
            Войти в Mixora
          </button>
        )}
        <Link className="text-button" to="/">
          На главную
        </Link>
      </div>
    </section>
  );
}
