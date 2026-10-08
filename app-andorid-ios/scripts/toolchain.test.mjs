import test from "node:test";
import assert from "node:assert/strict";
import { toolchainEnvironment } from "./toolchain.mjs";

test("Android commands select the installed macOS JDK/SDK without changing shell settings", () => {
  const input = { PATH: "/usr/bin", HOME: "/fixture" };
  const result = toolchainEnvironment(input, {
    platform: "darwin",
    userHome: "/fixture",
    exists: () => true,
  });
  assert.equal(
    result.JAVA_HOME,
    "/opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home",
  );
  assert.equal(result.ANDROID_HOME, "/fixture/Library/Android/sdk");
  assert.equal(
    result.NDK_HOME,
    "/fixture/Library/Android/sdk/ndk/28.2.13676358",
  );
  assert.equal(input.JAVA_HOME, undefined);
  assert.equal(result.HOME, "/fixture");
});

test("explicit SDK/JDK/NDK paths always win; other systems are never assigned Mac paths", () => {
  const input = {
    JAVA_HOME: "/custom/jdk",
    ANDROID_HOME: "/custom/sdk",
    NDK_HOME: "/custom/ndk",
  };
  assert.deepEqual(
    toolchainEnvironment(input, {
      platform: "darwin",
      userHome: "/fixture",
      exists: () => true,
    }),
    input,
  );
  assert.deepEqual(
    toolchainEnvironment(
      {},
      { platform: "linux", userHome: "/fixture", exists: () => true },
    ),
    {},
  );
  assert.deepEqual(
    toolchainEnvironment(
      {},
      { platform: "darwin", userHome: "/fixture", exists: () => false },
    ),
    {},
  );
});
