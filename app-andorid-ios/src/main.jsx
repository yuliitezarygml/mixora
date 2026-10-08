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
  static getDerivedStateFromError() {
    return { error: true };
  }
  render() {
    return this.state.error ? (
      <main className="error-screen">
        <h1>Не удалось открыть Mixora</h1>
        <button onClick={() => location.reload()}>Перезагрузить</button>
      </main>
    ) : (
      this.props.children
    );
  }
}

if (isTauri()) configureClientRuntime(createMobileAPI(invoke));
const root =
  import.meta.hot?.data.root ?? createRoot(document.getElementById("root"));
if (import.meta.hot) import.meta.hot.data.root = root;
root.render(
  <ErrorBoundary>
    <HashRouter>
      <AppProvider>
        <App />
      </AppProvider>
    </HashRouter>
  </ErrorBoundary>,
);
