import test from "node:test";
import assert from "node:assert/strict";
import { isDiscoveryPath, pageKind, originalRoutes } from "./routes.js";
test("every original route has a deliberate page family", () => {
  for (const route of originalRoutes) {
    if (["/404", "/not-found", "/unavailable"].includes(route)) continue;
    assert.notEqual(pageKind(route), "error", route);
  }
});
test("unknown routes render a not-found state", () =>
  assert.equal(pageKind("/does-not-exist"), "error"));
test("catalog overviews use the discovery screens", () => {
  for (const route of [
    "/genre",
    "/genre/artists",
    "/chart",
    "/chart/podcasts",
    "/mixes",
    "/kids",
    "/kids/category",
    "/non-music",
    "/non-music/category/albums",
    "/playlists",
    "/tag",
  ]) {
    assert.equal(pageKind(route), "browse", route);
    assert.equal(isDiscoveryPath(route), true, route);
  }
  for (const route of ["/concerts", "/video", "/users", "/music-history"]) {
    assert.equal(isDiscoveryPath(route), false, route);
  }
});
