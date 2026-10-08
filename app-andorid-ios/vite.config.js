import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import { sharedStyleTags } from "./scripts/sharedStyles.mjs";

const appRoot = fileURLToPath(new URL(".", import.meta.url));
const clientRoot = fileURLToPath(new URL("../mixora-client", import.meta.url));
const workspace = fileURLToPath(new URL("..", import.meta.url));
const port = 5176;

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, appRoot, "TAURI_");
  const config = JSON.parse(readFileSync(`${appRoot}/api.json`, "utf8"));
  const apiOrigin = process.env.MIXORA_API_URL || config.serverUrl;
  const host = process.env.TAURI_DEV_HOST || env.TAURI_DEV_HOST || "127.0.0.1";
  const proxy = {
    // Vite matches prefixes: "/api" also captures the frontend api.json module.
    // Keep server routes separate from the central, bundled configuration file.
    "/api/": {
      target: apiOrigin,
      changeOrigin: true,
      ws: true,
      configure(server) {
        const stripLocalOrigin = (request) => {
          // Only this dev server's own origin is stripped, not arbitrary sites.
          if (
            [
              `http://${host}:${port}`,
              `http://localhost:${port}`,
              "http://127.0.0.1:4176",
            ].includes(request.getHeader("origin"))
          ) {
            request.removeHeader("origin");
          }
        };
        server.on("proxyReq", stripLocalOrigin);
        server.on("proxyReqWs", stripLocalOrigin);
      },
    },
  };
  return {
    root: appRoot,
    plugins: [
      react(),
      {
        name: "mixora-shared-reference-styles",
        transformIndexHtml: {
          order: "pre",
          handler: () =>
            sharedStyleTags(readFileSync(`${clientRoot}/index.html`, "utf8")),
        },
      },
    ],
    publicDir: `${clientRoot}/public`,
    resolve: {
      dedupe: ["react", "react-dom", "react-router", "react-router-dom"],
    },
    server: {
      host,
      port,
      strictPort: true,
      ...(host !== "127.0.0.1"
        ? { hmr: { protocol: "ws", host, port: 5177 } }
        : {}),
      fs: { allow: [workspace] },
      proxy,
      watch: { ignored: ["**/src-tauri/**"] },
    },
    preview: { proxy },
    build: { outDir: "dist", emptyOutDir: true, target: "es2022" },
    clearScreen: false,
  };
});
