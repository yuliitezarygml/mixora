import test from "node:test";
import assert from "node:assert/strict";
import { createNativeTransport } from "./nativeTransport.js";

test("native login uses a bounded IPC request, not renderer cookies or arbitrary headers", async () => {
  const calls = [];
  const transport = createNativeTransport(async (command, args) => {
    calls.push({ command, args });
    return { status: 200, body: '{"success":true,"data":{"id":"user-1"}}' };
  });
  const response = await transport.request("/api/v1/auth/login", {
    method: "POST", body: '{"email":"test@example.invalid","password":"fixture"}',
    credentials: "include", headers: { "Content-Type": "application/json", Cookie: "must-not-forward" },
  });
  assert.equal(response.status, 200);
  assert.equal((await response.json()).data.id, "user-1");
  assert.deepEqual(calls[0], { command: "api_request", args: { request: {
    path: "/api/v1/auth/login", method: "POST",
    body: '{"email":"test@example.invalid","password":"fixture"}',
  } } });
});

test("204 logout produces a valid empty response", async () => {
  const transport = createNativeTransport(async () => ({ status: 204, body: "" }));
  const response = await transport.request("/api/v1/auth/logout", { method: "POST" });
  assert.equal(response.status, 204);
  assert.equal(await response.text(), "");
});

test("abort discards late native responses without applying stale account data", async () => {
  let finish;
  const transport = createNativeTransport(() => new Promise((resolve) => { finish = resolve; }));
  const controller = new AbortController();
  const request = transport.request("/api/v1/me", { signal: controller.signal });
  await Promise.resolve(); // The IPC has started; cancellation is now in-flight.
  controller.abort();
  await assert.rejects(request, (error) => error.name === "AbortError");
  finish({ status: 200, body: '{"id":"old-account"}' });
});

test("pre-aborted requests never cross IPC", async () => {
  let calls = 0;
  const transport = createNativeTransport(async () => { calls += 1; });
  await assert.rejects(transport.request("/api/v1/me", { signal: AbortSignal.abort() }), (error) => error.name === "AbortError");
  assert.equal(calls, 0);
});

test("native foreground previews remain usable; authenticated proxy is explicitly pending", async () => {
  const transport = createNativeTransport(async () => { throw new Error("not used"); });
  const preview = { url: "https://cdn.example/preview.mp3", format: "progressive" };
  assert.equal(await transport.preparePlayback(preview), preview);
  await assert.rejects(transport.preparePlayback({ url: "/api/v1/media/stream?url=fixture" }), /нативный плеер/i);
  assert.equal(transport.createPlaybackSocket(), null);
});
