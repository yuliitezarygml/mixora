const { app, BrowserWindow, shell } = require("electron");
const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");
const {
  DESKTOP_ORIGIN,
  isMixoraDevServerResponse,
  listenOnDesktopOrigin,
} = require("./origin.cjs");
const { isHttpProxyPath } = require("./routing.cjs");
const { contentSecurityPolicy } = require("./csp.cjs");
let server, window, origin;
const root = path.resolve(__dirname, "../dist");
const types = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript",
  ".css": "text/css",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".woff2": "font/woff2",
  ".woff": "font/woff",
  ".jpeg": "image/jpeg",
  ".png": "image/png",
  ".mp4": "video/mp4",
  ".webm": "video/webm",
};
function respond(req, res) {
  let url;
  try {
    url = new URL(req.url, "http://localhost");
  } catch {
    res.writeHead(400);
    res.end();
    return;
  }
  if (isHttpProxyPath(url.pathname)) {
    const headers = { ...req.headers, host: "127.0.0.1:8080" };
    if (headers.origin === origin) delete headers.origin;
    const upstream = http.request(
      {
        hostname: "127.0.0.1",
        port: 8080,
        path: req.url,
        method: req.method,
        headers,
      },
      (r) => {
        res.writeHead(r.statusCode, r.headers);
        r.pipe(res);
      },
    );
    upstream.on("error", () => {
      if (!res.headersSent)
        res.writeHead(502, { "Content-Type": "application/json" });
      res.end('{"error":"Backend unavailable"}');
    });
    req.pipe(upstream);
    return;
  }
  if (!["GET", "HEAD"].includes(req.method)) {
    res.writeHead(405);
    res.end();
    return;
  }
  let file;
  try {
    file = path.resolve(root, "." + decodeURIComponent(url.pathname));
  } catch {
    res.writeHead(400);
    res.end();
    return;
  }
  if (file !== root && !file.startsWith(root + path.sep)) {
    res.writeHead(403);
    res.end();
    return;
  }
  if (!path.extname(file)) file = path.join(root, "index.html");
  fs.stat(file, (err, stat) => {
    if (err || !stat.isFile()) {
      res.writeHead(404);
      res.end("Not found");
      return;
    }
    const headers = {
      "Content-Type": types[path.extname(file)] || "application/octet-stream",
      "Content-Security-Policy": contentSecurityPolicy,
      "X-Content-Type-Options": "nosniff",
      "Accept-Ranges": "bytes",
    };
    let start = 0,
      end = stat.size - 1,
      status = 200;
    if (req.headers.range) {
      const match = /^bytes=(\d+)-(\d*)$/.exec(req.headers.range);
      if (!match) {
        res.writeHead(416);
        res.end();
        return;
      }
      start = Number(match[1]);
      end = match[2] ? Math.min(Number(match[2]), end) : end;
      if (start > end) {
        res.writeHead(416, { "Content-Range": `bytes */${stat.size}` });
        res.end();
        return;
      }
      status = 206;
      headers["Content-Range"] = `bytes ${start}-${end}/${stat.size}`;
    }
    headers["Content-Length"] = end - start + 1;
    res.writeHead(status, headers);
    if (req.method === "HEAD") {
      res.end();
      return;
    }
    fs.createReadStream(file, { start, end })
      .on("error", () => res.destroy())
      .pipe(res);
  });
}
function probe(url) {
  return new Promise((resolve) => {
    const req = http.get(url, (res) => {
      res.resume();
      resolve(isMixoraDevServerResponse(res));
    });
    req.on("error", () => resolve(false));
    req.setTimeout(800, () => {
      req.destroy();
      resolve(false);
    });
  });
}
function openWindow(target) {
  const mac = process.platform === "darwin";
  window = new BrowserWindow({
    width: 1380,
    height: 900,
    minWidth: 768,
    minHeight: 650,
    show: false,
    title: "Mixora",
    backgroundColor: "#0d0d0d",
    titleBarStyle: mac ? "hidden" : "default",
    trafficLightPosition: mac ? { x: 16, y: 12 } : undefined,
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      autoplayPolicy: "no-user-gesture-required",
      preload: path.join(__dirname, "preload.cjs"),
    },
  });
  window.once("ready-to-show", () => window.show());
  window.webContents.setWindowOpenHandler(({ url }) => {
    try {
      if (new URL(url).protocol === "https:") shell.openExternal(url);
    } catch {}
    return { action: "deny" };
  });
  window.webContents.on("will-navigate", (event, url) => {
    if (new URL(url).origin !== origin) event.preventDefault();
  });
  window.loadURL(target);
}
app.whenReady().then(async () => {
  if (await probe(`${DESKTOP_ORIGIN}/`)) {
    origin = DESKTOP_ORIGIN;
    openWindow(origin);
    return;
  }
  if (!fs.existsSync(path.join(root, "index.html"))) {
    console.error(
      "Start the interface with npm run dev, or run npm run build before the desktop app.",
    );
    app.quit();
    return;
  }
  server = http.createServer(respond);
  server.on("upgrade", (req, socket, head) => {
    let target;
    try {
      target = new URL(req.url, "http://localhost");
    } catch {
      socket.destroy();
      return;
    }
    if (!target.pathname.startsWith("/api/v1/")) {
      socket.destroy();
      return;
    }
    const headers = { ...req.headers, host: "127.0.0.1:8080" };
    if (headers.origin === origin) delete headers.origin;
    const upstream = http.request({
      hostname: "127.0.0.1",
      port: 8080,
      path: req.url,
      method: req.method,
      headers,
    });
    upstream.on("upgrade", (res, upSocket, upHead) => {
      const lines = [`HTTP/1.1 ${res.statusCode} ${res.statusMessage}`];
      for (const [key, value] of Object.entries(res.headers)) {
        const list = Array.isArray(value) ? value : [value];
        for (const item of list) if (item) lines.push(`${key}: ${item}`);
      }
      socket.write(lines.join("\r\n") + "\r\n\r\n");
      if (upHead.length) socket.write(upHead);
      if (head.length) upSocket.write(head);
      upSocket.pipe(socket);
      socket.pipe(upSocket);
    });
    upstream.on("error", () => socket.destroy());
    upstream.on("response", (res) => {
      socket.write(`HTTP/1.1 ${res.statusCode} ${res.statusMessage}\r\n\r\n`);
      res.pipe(socket);
    });
    upstream.end();
  });
  try {
    origin = await listenOnDesktopOrigin(server);
    openWindow(origin);
  } catch (error) {
    console.error(
      `Could not start Mixora at ${DESKTOP_ORIGIN}: ${error.message}`,
    );
    app.quit();
  }
});
app.on("window-all-closed", () => app.quit());
app.on("before-quit", () => server?.close());
