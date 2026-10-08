import test from "node:test";
import assert from "node:assert/strict";
import { createServer as createHTTPServer } from "node:http";
import { once } from "node:events";
import { createServer as createViteServer } from "vite";
import viteConfig from "../vite.config.js";

test("dev startup loads api.json as a module while API requests reach the backend", async (t) => {
  const requests = [];
  const backend = createHTTPServer((request, response) => {
    requests.push(request.url);
    response.setHeader("Content-Type", "application/json");
    response.statusCode = request.url === "/api/v1/auth/session" ? 401 : 404;
    response.end(JSON.stringify({ success: false, error: "Not authenticated" }));
  });
  backend.listen(0, "127.0.0.1");
  await once(backend, "listening");
  t.after(() => new Promise((resolve) => backend.close(resolve)));

  const previousOrigin = process.env.MIXORA_API_URL;
  let config;
  try {
    process.env.MIXORA_API_URL = `http://127.0.0.1:${backend.address().port}`;
    config = viteConfig({ mode: "test" });
  } finally {
    if (previousOrigin === undefined) delete process.env.MIXORA_API_URL;
    else process.env.MIXORA_API_URL = previousOrigin;
  }
  assert.equal(config.preview.proxy, config.server.proxy);
  const vite = await createViteServer({
    ...config,
    configFile: false,
    server: {
      ...config.server,
      host: "127.0.0.1",
      port: 0,
      strictPort: false,
      hmr: false,
      watch: null,
    },
  });
  t.after(() => vite.close());
  await vite.listen();
  const origin = `http://127.0.0.1:${vite.httpServer.address().port}`;

  const settings = await fetch(`${origin}/api.json?import`);
  assert.equal(settings.status, 200, "api.json must not be proxied to the backend");
  assert.match(settings.headers.get("content-type"), /javascript/);
  assert.match(await settings.text(), /export default/);
  assert.deepEqual(requests, [], "frontend modules never reach the backend");

  const session = await fetch(`${origin}/api/v1/auth/session`);
  assert.equal(session.status, 401);
  assert.deepEqual(requests, ["/api/v1/auth/session"]);
});
