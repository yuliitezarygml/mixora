const assert = require("node:assert/strict");
const test = require("node:test");
const { isHttpProxyPath } = require("./routing.cjs");

test("desktop server proxies API and every health endpoint", () => {
  assert.equal(isHttpProxyPath("/api/v1/session"), true);
  assert.equal(isHttpProxyPath("/health"), true);
  assert.equal(isHttpProxyPath("/health/live"), true);
  assert.equal(isHttpProxyPath("/health/ready"), true);
});

test("desktop server does not proxy similarly named UI paths", () => {
  assert.equal(isHttpProxyPath("/api/v10/session"), false);
  assert.equal(isHttpProxyPath("/healthcheck"), false);
  assert.equal(isHttpProxyPath("/collection"), false);
});
