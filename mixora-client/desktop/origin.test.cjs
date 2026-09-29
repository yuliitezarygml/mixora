const assert = require("node:assert/strict");
const http = require("node:http");
const test = require("node:test");
const {
  DESKTOP_HOST,
  DESKTOP_PORT,
  DESKTOP_ORIGIN,
  isMixoraDevServerResponse,
  listenOnDesktopOrigin,
} = require("./origin.cjs");

function close(server) {
  return new Promise((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
  });
}

async function reservePort() {
  const server = http.createServer();
  const origin = await listenOnDesktopOrigin(server, { port: 0 });
  const port = Number(new URL(origin).port);
  await close(server);
  return port;
}

test("desktop production origin has a fixed loopback port", () => {
  assert.equal(DESKTOP_HOST, "127.0.0.1");
  assert.equal(DESKTOP_PORT, 5174);
  assert.equal(DESKTOP_ORIGIN, "http://127.0.0.1:5174");
});

test("desktop only reuses a verified Mixora dev server", () => {
  assert.equal(
    isMixoraDevServerResponse({
      statusCode: 200,
      headers: { "x-mixora-dev-server": "1" },
    }),
    true,
  );
  assert.equal(
    isMixoraDevServerResponse({ statusCode: 200, headers: {} }),
    false,
  );
  assert.equal(
    isMixoraDevServerResponse({
      statusCode: 503,
      headers: { "x-mixora-dev-server": "1" },
    }),
    false,
  );
});

test("desktop server keeps the same origin across restarts", async (t) => {
  const port = await reservePort();

  for (let attempt = 0; attempt < 2; attempt += 1) {
    const server = http.createServer((_request, response) => response.end());
    t.after(async () => {
      if (server.listening) await close(server);
    });
    const origin = await listenOnDesktopOrigin(server, { port });
    assert.equal(origin, `http://127.0.0.1:${port}`);
    await close(server);
  }
});

test("desktop server fails instead of silently changing origin", async (t) => {
  const blocker = http.createServer();
  t.after(async () => {
    if (blocker.listening) await close(blocker);
  });
  const blockedOrigin = await listenOnDesktopOrigin(blocker, { port: 0 });
  const port = Number(new URL(blockedOrigin).port);
  const server = http.createServer();

  await assert.rejects(listenOnDesktopOrigin(server, { port }), {
    code: "EADDRINUSE",
  });
  assert.equal(server.listening, false);

  await close(blocker);
});
