import { useEffect, useRef, useState } from "react";
import { api, type Encoder, type Estimate, type Settings as S } from "@/lib/api";
import { useStore } from "@/lib/store";
import { AboutSection } from "./sections/About";
import { AISection } from "./sections/AI";
import { AppSection } from "./sections/App";
import { AudioSection } from "./sections/Audio";
import { ClipsSection } from "./sections/Clips";
import { FFmpegSection } from "./sections/FFmpeg";
import { HotkeysSection } from "./sections/Hotkeys";
import { SharingSection } from "./sections/Sharing";
import { StorageSection } from "./sections/Storage";
import { VideoSection } from "./sections/Video";

export const sections = [
  ["capture", "Video"],
  ["buffer", "Clips"],
  ["audio", "Audio"],
  ["hotkeys", "Hotkeys"],
  ["storage", "Storage"],
  ["sharing", "Sharing"],
  ["ai", "AI"],
  ["app", "App"],
  ["ffmpeg", "FFmpeg"],
  ["about", "About"],
] as const;

export function Settings() {
  const s = useStore((st) => st.settings)!;
  const save = useStore((st) => st.setSettings);
  const boot = useStore((st) => st.boot)!;
  const [est, setEst] = useState<Estimate | null>(null);
  const [encoders, setEncoders] = useState<Encoder[]>(boot.encoders ?? []);
  const scroller = useRef<HTMLDivElement>(null);
  const jump = useStore((st) => (st.view.name === "settings" ? st.view.section : undefined));
  const set = (p: Partial<S>) => save({ ...s, ...p });

  useEffect(() => {
    if (jump) document.getElementById(`s-${jump}`)?.scrollIntoView();
  }, [jump]);

  useEffect(() => {
    api
      .estimate(s)
      .then(setEst)
      .catch(() => {});
  }, [s.quality, s.fps, s.resolution, s.codec, s.bufferSeconds, s.monitor, s.captureMic, s.captureSystemAudio]);

  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const onScroll = () => {
      let cur: string = sections[0][0];
      for (const [id] of sections) {
        const node = document.getElementById(`s-${id}`);
        if (node && node.offsetTop - el.scrollTop < 120) cur = id;
      }
      // last sections are too short to ever reach the top
      if (el.scrollTop + el.clientHeight >= el.scrollHeight - 4) cur = sections[sections.length - 1][0];
      useStore.setState({ settingsActive: cur });
    };
    el.addEventListener("scroll", onScroll);
    return () => el.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <div ref={scroller} className="h-full overflow-y-auto">
      <div className="max-w-[760px] px-10 py-6">
        <VideoSection s={s} set={set} est={est} encoders={encoders} />
        <ClipsSection s={s} set={set} est={est} />
        <AudioSection s={s} set={set} />
        <HotkeysSection s={s} set={set} />
        <StorageSection s={s} set={set} />
        <SharingSection s={s} set={set} />
        <AISection s={s} set={set} />
        <AppSection s={s} set={set} />
        <FFmpegSection s={s} set={set} encoders={encoders} setEncoders={setEncoders} />
        <AboutSection s={s} set={set} version={boot.version} />
      </div>
    </div>
  );
}
