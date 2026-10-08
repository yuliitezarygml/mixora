import test from "node:test";
import assert from "node:assert/strict";
import { normalizeTaste, tasteContext } from "./tasteProfile.js";
import { buildWaveRequest } from "./waveRequest.js";
import { recommendationFingerprint } from "./recommendedPlaylists.js";

test("initial interests rotate artist seeds and never fabricate liked tracks", () => {
  const taste = {
    artists: ["Tycho", "RAC", "Miyagi", "ODESZA", "Daft Punk"],
    genres: ["electronic"],
    completed: true,
  };
  const request = buildWaveRequest({ taste, round: 1 });
  assert.deepEqual(request.context, { artist: "RAC", genre: "electronic" });
  assert.deepEqual(request.likes, []);
  assert.deepEqual(
    buildWaveRequest({
      taste,
      library: { likes: [{ id: "1", source: "youtube", artist: "New taste" }] },
    }).context,
    {},
  );
  assert.deepEqual(
    buildWaveRequest({ taste, context: { artist: "Station" } }).context,
    { artist: "Station" },
  );
  assert.deepEqual(tasteContext(taste, -1), {
    artist: "Daft Punk",
    genre: "electronic",
  });
  assert.notEqual(
    recommendationFingerprint({ taste }),
    recommendationFingerprint({ taste: { ...taste, artists: ["Changed"] } }),
  );
  assert.deepEqual(
    normalizeTaste({ artists: ["Tycho", null, "Tycho"], genres: ["unknown"] }),
    { artists: ["Tycho"], genres: [], completed: false },
  );
});
