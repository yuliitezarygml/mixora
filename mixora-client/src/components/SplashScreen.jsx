import { useEffect, useState } from "react";
import { useApp } from "../state/context.js";

const themes = {
  dark: "SplashScreen_root_dark__0OcZj",
  light: "SplashScreen_root_light__XAJTf",
};

export default function SplashScreen() {
  const { settings } = useApp();
  const [phase, setPhase] = useState("play");
  const reduced =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const theme = settings.theme === "light" ? "light" : "dark";

  useEffect(() => {
    if (!settings.animation || reduced) {
      setPhase("gone");
      return;
    }
    const timer = window.setTimeout(() => setPhase("hide"), 5000);
    return () => window.clearTimeout(timer);
  }, [reduced, settings.animation]);

  if (phase === "gone" || !settings.animation || reduced) return null;

  return (
    <div
      className={`SplashScreen_root__3jzFk ${themes[theme]} ${phase === "hide" ? "SplashScreen_root_hidden__BO7tp" : ""}`}
      onAnimationEnd={() => {
        if (phase === "hide") setPhase("gone");
      }}
    >
      <video
        className="SplashScreen_video__wFSy5"
        autoPlay
        muted
        playsInline
        onEnded={() => setPhase("hide")}
        onError={() => setPhase("gone")}
      >
        <source
          src={`/assets/media/splash_screen/splash_screen_${theme}.webm`}
          type="video/webm"
        />
        <source
          src={`/assets/media/splash_screen/splash_screen_${theme}.mp4`}
          type="video/mp4"
        />
      </video>
    </div>
  );
}
