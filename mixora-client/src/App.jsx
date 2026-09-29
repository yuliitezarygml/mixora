import { useEffect, useState } from "react";
import { Link, NavLink, useLocation, useNavigate } from "react-router-dom";
import { useApp } from "./state/context.js";
import { isDiscoveryPath, pageKind } from "./lib/routes.js";
import Icon from "./components/Icon.jsx";
import { IconButton, Empty } from "./components/Primitives.jsx";
import Player, { PlayerPanel } from "./components/Player.jsx";
import WaveSettings from "./components/WaveSettings.jsx";
import AuthModal from "./components/AuthModal.jsx";
import Home from "./pages/Home.jsx";
import Search from "./pages/Search.jsx";
import Collection, { CreatePlaylist } from "./pages/Collection.jsx";
import Details from "./pages/Details.jsx";
import Browse from "./pages/Browse.jsx";
import Discovery from "./pages/Discovery.jsx";
import Settings from "./pages/Settings.jsx";
import AuthAction from "./pages/AuthAction.jsx";
import Sidebar, { navigation } from "./components/Sidebar.jsx";
import SplashScreen from "./components/SplashScreen.jsx";
export default function App() {
  const a = useApp(),
    location = useLocation(),
    navigate = useNavigate();
  const [create, setCreate] = useState(false),
    [menu, setMenu] = useState(false);
  const kind = pageKind(location.pathname);
  useEffect(() => {
    document.querySelector(".page-scroll")?.scrollTo({ top: 0 });
    setMenu(false);
  }, [location.pathname, location.search]);
  useEffect(() => {
    const handler = (e) => {
      if (
        e.code === "Space" &&
        !["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes(e.target.tagName) &&
        !e.target.isContentEditable &&
        !document.querySelector("dialog[open]")
      ) {
        e.preventDefault();
        a.toggle();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [a.current, a.playing]);
  let content =
    kind === "home" ? (
      <Home />
    ) : kind === "search" ? (
      <Search />
    ) : kind === "collection" ? (
      <Collection />
    ) : kind === "details" ? (
      <Details />
    ) : kind === "settings" ? (
      <Settings />
    ) : kind === "browse" ? (
      isDiscoveryPath(location.pathname) ? (
        <Discovery />
      ) : (
        <Browse />
      )
    ) : kind === "oauth" ? (
      <div className="page-padding">
        <Empty
          title="Вход в Mixora"
          text="Используйте свой аккаунт Mixora для прослушивания музыки."
          action={
            <button className="primary" onClick={() => a.setAuthOpen(true)}>
              Войти
            </button>
          }
        />
      </div>
    ) : kind === "auth_action" ? (
      <AuthAction
        action={location.pathname === "/verify-email" ? "verify" : "reset"}
      />
    ) : kind === "landing" ? (
      <div className="landing">
        <Icon name="musicLogo" size={100} />
        <h1>
          Вся ваша музыка.
          <br />
          Ваш ритм.
        </h1>
        <p>Находите новое и собирайте любимое в Mixora.</p>
        <Link className="primary" to="/">
          Открыть музыку
        </Link>
      </div>
    ) : (
      <div className="page-padding">
        <Empty
          icon="unavailable_xl"
          title={
            location.pathname === "/unavailable"
              ? "Сервис временно недоступен"
              : "Страница не найдена"
          }
          text="Вернитесь на главную, чтобы продолжить слушать музыку."
          action={
            <Link className="primary" to="/">
              На главную
            </Link>
          }
        />
      </div>
    );
  return (
    <div className="app-shell CommonLayout_root__WC_W1 DefaultLayout_root__7J0wo">
      <SplashScreen />
      <Sidebar onCreate={() => setCreate(true)} />
      <main
        className={`main-surface CommonLayout_content__zy_Ja route-${kind}`}
      >
        {kind !== "home" &&
          kind !== "search" &&
          location.pathname !== "/collection" && (
            <header className="topbar">
              <div className="history-controls">
                <IconButton
                  icon="arrowLeft_xs"
                  label="Назад"
                  onClick={() => navigate(-1)}
                />
                <IconButton
                  icon="arrowRight_xs"
                  label="Вперёд"
                  onClick={() => navigate(1)}
                />
              </div>
            </header>
          )}
        <div className="page-scroll" id="main-content">
          {content}
        </div>
      </main>
      <Player />
      <PlayerPanel />
      {a.authOpen && <AuthModal />}
      {a.waveSettingsOpen && <WaveSettings />}
      {create && <CreatePlaylist onClose={() => setCreate(false)} />}
      <nav className="mobile-nav" aria-label="Мобильное меню">
        {navigation
          .filter(([, to]) => ["/", "/search", "/collection"].includes(to))
          .map(([label, to, icon]) => (
            <NavLink end={to === "/"} to={to} key={to}>
              <Icon name={`${icon}_m`} />
              <span>{label}</span>
            </NavLink>
          ))}
        <NavLink to="/settings">
          <Icon name="settings_xs" />
          <span>Настройки</span>
        </NavLink>
      </nav>
      {a.notice && (
        <div className="toast" role="status">
          {a.notice}
        </div>
      )}
    </div>
  );
}
