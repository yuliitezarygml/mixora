import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

export function toolchainEnvironment(
  input,
  {
    platform = process.platform,
    userHome = homedir(),
    exists = existsSync,
  } = {},
) {
  const environment = { ...input };
  if (platform !== "darwin") return environment;
  const jdk = "/opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home";
  const sdk = `${userHome}/Library/Android/sdk`;
  if (!environment.JAVA_HOME && exists(`${jdk}/bin/java`))
    environment.JAVA_HOME = jdk;
  if (!environment.ANDROID_HOME && exists(`${sdk}/platform-tools/adb`))
    environment.ANDROID_HOME = sdk;
  const ndk = `${environment.ANDROID_HOME}/ndk/28.2.13676358`;
  if (
    !environment.NDK_HOME &&
    environment.ANDROID_HOME &&
    exists(`${ndk}/source.properties`)
  )
    environment.NDK_HOME = ndk;
  return environment;
}

if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const cli = fileURLToPath(
    new URL("../node_modules/@tauri-apps/cli/tauri.js", import.meta.url),
  );
  const child = spawn(process.execPath, [cli, ...process.argv.slice(2)], {
    stdio: "inherit",
    env: toolchainEnvironment(process.env),
  });
  child.on("error", (error) => {
    console.error(error.message);
    process.exitCode = 1;
  });
  child.on("exit", (code) => {
    process.exitCode = code ?? 1;
  });
  for (const signal of ["SIGINT", "SIGTERM"])
    process.on(signal, () => child.kill(signal));
}
