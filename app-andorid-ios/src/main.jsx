import React from "react";
import { createRoot } from "react-dom/client";
import { HashRouter } from "react-router-dom";
import { invoke, isTauri } from "@tauri-apps/api/core";
import App from "../../mixora-client/src/App.jsx";
import { AppProvider } from "../../mixora-client/src/state/AppContext.jsx";
import { configureClientRuntime } from "../../mixora-client/src/lib/clientRuntime.js";
import { createMobileAPI } from "./api.js";
import "../../mixora-client/src/styles/global.css";
import "../../mixora-client/src/styles/integration.css";
import "./native.css";

class ErrorBoundary extends React.Component {
  state = { error: false };
  static getDerivedStateFromError() { return { error: true }; }
  render() {
    return this.state.error ? (
      <main className="error-screen"><h1>Не удалось открыть Mixora</h1><button onClick={() => location.reload()}>Перезагрузить</button></main>
    ) : this.props.children;
  }
}

function NativeApp() {
  const [ready, setReady] = React.useState(!isTauri());
  return ready ? (
    <HashRouter><AppProvider><App /></AppProvider></HashRouter>
  ) : (
    <main className="native-intro">
      <p className="native-label">Tauri 2 · тестовая сборка</p>
      <h1>Mixora Mobile</h1>
      <p>Общий интерфейс Mixora и подключение к вашему серверу.</p>
      <p>Аккаунт и поиск используют нативную сессию. После закрытия приложения потребуется повторный вход.</p>
      <p>Фоновый звук, потоки YouTube/VK/Bandcamp и синхронизация плеера между устройствами пока в разработке. Для них сохранён текущий ПК-клиент.</p>
      <button className="primary" onClick={() => setReady(true)}>Открыть приложение</button>
    </main>
  );
}

if (isTauri()) configureClientRuntime(createMobileAPI(invoke));
const root = import.meta.hot?.data.root ?? createRoot(document.getElementById("root"));
if (import.meta.hot) import.meta.hot.data.root = root;
root.render(<ErrorBoundary><NativeApp /></ErrorBoundary>);
