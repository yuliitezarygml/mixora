import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
const proxy = {
  "/api": {
    target: "http://127.0.0.1:8080",
    changeOrigin: true,
    ws: true,
    configure(server) {
      const strip = (req) => {
        if (
          [
            "http://localhost:5174",
            "http://127.0.0.1:5174",
            "http://localhost:4174",
            "http://127.0.0.1:4174",
          ].includes(req.getHeader("origin"))
        )
          req.removeHeader("origin");
      };
      server.on("proxyReq", strip);
      server.on("proxyReqWs", strip);
    },
  },
  "/health": { target: "http://127.0.0.1:8080", changeOrigin: true },
};
export default defineConfig({
  plugins: [react()],
  server: { proxy, headers: { "X-Mixora-Dev-Server": "1" } },
  preview: { proxy },
  build: { target: "es2022" },
});
