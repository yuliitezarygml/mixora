import test from "node:test";
import assert from "node:assert/strict";
import { apiSettings, api, createMobileAPI } from "./api.js";
import { configureClientRuntime } from "../../mixora-client/src/lib/clientRuntime.js";

test("one mobile API facade uses the shared contract and central server configuration", async () => {
  assert.equal(apiSettings.apiPrefix, "/api/v1");
  assert.equal(apiSettings.playbackSocketPath, `${apiSettings.apiPrefix}/playback/ws`);
  for (const origin of [apiSettings.serverUrl, apiSettings.androidEmulatorUrl]) {
    const url = new URL(origin);
    assert.equal(url.pathname, "/");
    assert.equal(url.username, "");
    assert.equal(url.password, "");
  }
  const paths = [];
  const restore = configureClientRuntime(createMobileAPI(async (_, { request }) => {
    paths.push(request.path);
    return { status: 200, body: '{"success":true,"data":[]}' };
  }));
  try {
    assert.deepEqual(await api("/library/tracks"), []);
    assert.deepEqual(paths, [`${apiSettings.apiPrefix}/library/tracks`]);
  } finally { restore(); }
});
