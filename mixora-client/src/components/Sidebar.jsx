import { useEffect, useRef, useState } from "react";
import { Link, NavLink, useLocation } from "react-router-dom";
import { useApp } from "../state/context.js";
import Icon from "./Icon.jsx";
import { IconButton } from "./Primitives.jsx";
import AccountMenu from "./AccountMenu.jsx";
export const navigation = [
  ["Поиск", "/search", "search"],
  ["Главная", "/", "home"],
  ["Детям", "/kids", "kids"],
  ["Коллекция", "/collection", "collections"],
];
export default function Sidebar({ onCreate }) {
  const app = useApp();
  const location = useLocation();
  const [collapsed, setCollapsed] = useState(false);
  return (
    <aside
      className={`sidebar Navbar_root__chF4R DefaultLayout_navbar__LIQWG ${collapsed ? "Navbar_root_collapsed__pozJX" : ""}`}
    >
      <div className="NavbarDesktop_root__scYzp">
        <div className="NavbarDesktop_logoWrapper__89ce6">
          <Link
            className="NavbarDesktop_logoLink__KR0Dk"
            to="/"
            aria-label="Mixora — главная"
          >
            <div className="NavbarDesktop_logo__Z4jGx brand">
              <Icon name="musicLogo" size={32} />
              {!collapsed && <span>Mixora</span>}
            </div>
          </Link>
          <button
            className="NavbarDesktop_collapseButton__XQh9d"
            onClick={() => setCollapsed(!collapsed)}
            aria-label={collapsed ? "Развернуть меню" : "Свернуть меню"}
            aria-expanded={!collapsed}
          >
            <Icon
              name={collapsed ? "arrowRight_xxs" : "arrowLeft_xxs"}
              size={16}
            />
          </button>
        </div>
        <div className="NavbarDesktop_scrollableContainer__HLc9D">
          <div className="NavbarDesktop_scrollableContent__OyU4P">
            <nav
              className={`NGdj0oZ2Bt8qdZhP2Tzt QilmoKKJwk6f0BdkYgrA NavbarDesktop_navigation__dLUGW ${collapsed ? "rece5errcONnjJeX0YW8" : ""}`}
              aria-label="Главное меню"
            >
              <ol className="yuyI2hMAT7qyL1N14MAQ xfFtKQpgAYvC2jI1tBtS">
                {navigation.map(([label, to, icon]) => {
                  const selected =
                    to === "/"
                      ? location.pathname === "/"
                      : location.pathname.startsWith(to);
                  return (
                    <li
                      key={to}
                      className={`Bp1d3U6W8Nrbqi3MRQS_ hYfgO_Y8c4rrQsZJWTDZ H4trq_Zx2d9qOnQgxmxr ${collapsed ? "Q3gGGaIXiJ_oQTiVZBfl" : ""}`}
                    >
                      <NavLink
                        end={to === "/"}
                        to={to}
                        title={collapsed ? label : undefined}
                        className={`A4bDkbQHkwWNGqxO9qhW Xx9Tg5ugzg1pkf8Zh421 ${selected ? "mAd9pgMkWVX5ktCgYINQ" : ""}`}
                      >
                        <div className="zpkgiiHgDpbBThy6gavq">
                          <Icon
                            name={`${icon}${selected ? "_selected" : ""}_m`}
                            size={24}
                          />
                        </div>
                        {!collapsed && (
                          <div
                            className={`ZrkG6gNYcr4h3hfkhyT1 ${selected ? "xENlRAFvRskKnt8LUObC" : ""}`}
                          >
                            <span>{label}</span>
                          </div>
                        )}
                      </NavLink>
                    </li>
                  );
                })}
              </ol>
            </nav>
            <div className="NavbarDesktop_pinsList___jXIM sidebar-pins">
              <Link
                to="/mymusic/favorite_tracks"
                className="sidebar-pin"
                title="Мне нравится"
              >
                <span className="pin-cover liked-pin">
                  <Icon name="liked_m" size={24} />
                </span>
                {!collapsed && <span>Мне нравится</span>}
              </Link>
              {(app.library.pins || [])
                .map(
                  (id) =>
                    app.library.playlists.find((p) => p.id === id) ||
                    app.library.savedPlaylists.find((p) => p.id === id),
                )
                .filter(Boolean)
                .map((p) => (
                  <PinnedPlaylist
                    key={p.id}
                    playlist={p}
                    collapsed={collapsed}
                  />
                ))}
              <button
                className="sidebar-pin create-pin"
                onClick={onCreate}
                title="Создать плейлист"
              >
                <span className="pin-cover">
                  <Icon name="add_xxs" size={20} />
                </span>
                {!collapsed && <span>Создать плейлист</span>}
              </button>
            </div>
          </div>
        </div>
        <AccountMenu collapsed={collapsed} />
      </div>
    </aside>
  );
}

function PinnedPlaylist({ playlist, collapsed }) {
  const app = useApp();
  const [menu, setMenu] = useState(false);
  const root = useRef(null);
  useEffect(() => {
    if (!menu) return;
    const close = (event) => {
      if (!root.current?.contains(event.target)) setMenu(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [menu]);
  return (
    <div className="sidebar-pin-row" ref={root}>
      <Link
        className="sidebar-pin"
        to={
          app.library.playlists.some((item) => item.id === playlist.id)
            ? `/playlist?id=${playlist.id}`
            : `/playlist?id=${encodeURIComponent(playlist.id)}`
        }
        title={playlist.name}
      >
        <span className="pin-cover">
          {playlist.tracks?.[0]?.artwork ? (
            <img src={playlist.tracks[0].artwork} alt="" />
          ) : (
            <Icon name="playlist_xs" />
          )}
        </span>
        {!collapsed && <span>{playlist.name}</span>}
      </Link>
      {!collapsed && (
        <button
          className="pin-more"
          aria-label={`Действия: ${playlist.name}`}
          aria-expanded={menu}
          onClick={() => setMenu((value) => !value)}
        >
          <Icon name="more_xs" size={18} />
        </button>
      )}
      {menu && (
        <div className="tile-menu pin-menu" role="menu">
          <button
            role="menuitem"
            onClick={() => {
              app.togglePin(playlist.id);
              setMenu(false);
            }}
          >
            <Icon name="unpin_xxs" size={18} />
            Открепить
          </button>
        </div>
      )}
    </div>
  );
}
