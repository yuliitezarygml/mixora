const test = require("node:test");
const assert = require("node:assert/strict");
const { contentSecurityPolicy } = require("./csp.cjs");

test("desktop CSP permits provider audio and remote artwork", () => {
  const directives = Object.fromEntries(
    contentSecurityPolicy.split("; ").map((directive) => {
      const [name, ...sources] = directive.split(" ");
      return [name, sources];
    }),
  );

  assert.deepEqual(directives["img-src"], [
    "'self'",
    "https://*.sndcdn.com",
    "https://*.scdn.co",
    "https://*.ytimg.com",
    "https://*.ggpht.com",
    "https://*.bcbits.com",
    "https://*.userapi.com",
    "https://*.vkuseraudio.net",
    "https://*.okcdn.ru",
    "data:",
  ]);
  assert.deepEqual(directives["media-src"], [
    "'self'",
    "https://*.sndcdn.com",
    "https://playback.media-streaming.soundcloud.cloud",
    "https://*.scdn.co",
    "blob:",
  ]);
  assert.deepEqual(directives["connect-src"], [
    "'self'",
    "https://*.sndcdn.com",
    "https://playback.media-streaming.soundcloud.cloud",
    "https://*.scdn.co",
  ]);
  assert.deepEqual(directives["script-src"], ["'self'"]);
  assert.deepEqual(directives["object-src"], ["'none'"]);
});
