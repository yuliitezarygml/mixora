import { useEffect, useRef } from "react";
import { useApp } from "../state/context.js";
// Same fallback markup, dimensions, assets and class names as chunk 532.
export default function VibeBackground() {
  const { settings, playing } = useApp();
  const video = useRef(null);
  const base = `/assets/media/vibe_animation_fallback/vibe_animation_fallback_${settings.theme}`;
  useEffect(() => {
    if (video.current) video.current.playbackRate = playing ? 1 : 0.8;
  }, [playing, settings.animation]);
  return (
    <div
      className="VibeAnimation_root__UKMJy VibeAnimation_root_visible__S7kXl VibeBlock_vibeAnimation__XVEE6 vibe-background"
      aria-hidden="true"
    >
      {settings.animation ? (
        <video
          ref={video}
          width="1000"
          height="1000"
          preload="metadata"
          loop
          autoPlay
          muted
          playsInline
          disablePictureInPicture
          src={`${base}.mp4`}
          poster={`${base}.jpeg`}
        />
      ) : (
        <img width="1000" height="1000" src={`${base}.jpeg`} alt="" />
      )}
      <div />
    </div>
  );
}
