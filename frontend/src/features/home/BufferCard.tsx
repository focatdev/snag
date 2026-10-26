import { ArrowClockwise, CircleNotch, DownloadSimple, FolderOpen, Stop } from "@phosphor-icons/react";
import type { ReactNode } from "react";
import errorImg from "@/assets/error.jpg";
import liveImg from "@/assets/live.jpg";
import pausedImg from "@/assets/paused.jpg";
import setupImg from "@/assets/setup.jpg";
import waitingImg from "@/assets/waiting.jpg";
import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { api } from "@/lib/api";
import { bytes, clock } from "@/lib/format";
import { useStore } from "@/lib/store";

function Banner({ image, position = "50% 50%", children, footer }: { image: string; position?: string; children: ReactNode; footer?: ReactNode }) {
  return (
    <section className="relative overflow-hidden rounded-card bg-black text-white">
      <img src={image} alt="" className="absolute inset-0 h-full w-full object-cover opacity-60" style={{ objectPosition: position }} />
      <div className="relative flex min-h-[288px] flex-col p-7">{children}</div>
      {footer}
    </section>
  );
}

const ghostOnPhoto = "text-white/85 hover:bg-white/10 hover:text-white";

export function SetupCard() {
  const dl = useStore((s) => s.status?.download);
  const pct = dl && dl.total > 0 ? dl.done / dl.total : 0;
  return (
    <Banner image={setupImg} position="50% 40%">
      <p className="text-sm text-white/75">One step before your first clip</p>
      <h1 className="display mt-2 max-w-[560px] text-3xl leading-[1.1] tracking-[-0.02em]">Snag needs FFmpeg to record</h1>
      <p className="mt-2 max-w-[460px] text-sm text-white/75">About 100 MB. Pick where it goes in Settings, or use a copy you already have.</p>
      <div className="flex-1" />
      {dl?.active ? (
        <div className="max-w-[420px]">
          <div className="mb-2 flex justify-between text-xs text-white/75">
            <span>Downloading</span>
            <span>
              {bytes(dl.done)} of {bytes(dl.total)}
            </span>
          </div>
          <div className="h-1 overflow-hidden rounded-full bg-white/20">
            <div className="h-full bg-white transition-[width]" style={{ width: `${pct * 100}%` }} />
          </div>
        </div>
      ) : (
        <div className="flex items-center gap-2">
          <Button variant="primary" onClick={() => api.downloadFFmpeg().catch(() => {})}>
            <DownloadSimple size={16} />
            Download FFmpeg
          </Button>
          <Button
            variant="ghost"
            className={ghostOnPhoto}
            onClick={async () => {
              const p = await api.pickFFmpeg();
              const s = useStore.getState().settings;
              if (p && s) useStore.getState().setSettings({ ...s, ffmpegPath: p });
            }}
          >
            <FolderOpen size={16} />
            Use my own copy
          </Button>
          {dl?.error && <span className="ml-2 text-sm text-[#ffb4ab]">{dl.error}</span>}
        </div>
      )}
    </Banner>
  );
}

function lengthWords(sec: number) {
  if (sec === 60) return "minute";
  if (sec % 60 === 0) return `${sec / 60} minutes`;
  return `${sec} seconds`;
}

export function BufferCard() {
  const st = useStore((s) => s.status);
  const settings = useStore((s) => s.settings);
  if (!st || !settings) return null;
  const r = st.recorder;
  const target = r.bufferTarget || settings.bufferSeconds;
  const live = r.state === "buffering";
  const filling = live && r.bufferedSec < target - 0.5;
  const game = r.game && r.game !== "Desktop" ? r.game : "";

  let image = pausedImg;
  let position = "50% 45%";
  let chip: ReactNode = <span>Paused</span>;
  let title = "Capture is paused";
  let sub: ReactNode = "Nothing is being recorded";
  if (live) {
    image = liveImg;
    position = "50% 60%";
    chip = (
      <>
        <span className="live-dot h-[7px] w-[7px] rounded-full bg-live" />
        <span>Capturing</span>
      </>
    );
    title = game || "Ready to clip";
    sub = (
      <span className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <span className="flex items-center gap-2">
          <Kbd keys={settings.hotkeys.saveClip} />
          last {lengthWords(target)}
        </span>
        {settings.hotkeys.saveShort && (
          <span className="flex items-center gap-2">
            <Kbd keys={settings.hotkeys.saveShort} />
            last {Math.min(settings.shortSeconds, target)} seconds
          </span>
        )}
      </span>
    );
  } else if (r.state === "starting") {
    image = liveImg;
    position = "50% 60%";
    chip = (
      <>
        <CircleNotch size={12} className="animate-spin" />
        <span>Starting</span>
      </>
    );
    title = "Getting ready";
    sub = "Just a second";
  } else if (r.state === "error") {
    image = errorImg;
    chip = <span>Stopped</span>;
    title = "Capture stopped";
    sub = <span className="line-clamp-2 max-w-[560px]">{r.error}</span>;
  } else if (r.smartWaiting) {
    image = waitingImg;
    position = "50% 55%";
    chip = <span>Standing by</span>;
    title = "Waiting for a game";
    sub = "Starts on its own when you open one";
  }

  return (
    <Banner
      image={image}
      position={position}
      footer={
        filling ? (
          <div className="absolute inset-x-0 bottom-0 h-[3px] bg-white/15">
            <div className="h-full bg-white/90 transition-[width] duration-700" style={{ width: `${(r.bufferedSec / target) * 100}%` }} />
          </div>
        ) : null
      }
    >
      <div className="flex items-center justify-between">
        <div className="flex h-7 items-center gap-2 rounded-full bg-black/45 px-3 text-xs font-medium backdrop-blur-sm">{chip}</div>
        {filling && (
          <span className="text-xs text-white/70">
            {clock(r.bufferedSec)} of {clock(target)} so far
          </span>
        )}
      </div>
      <div className="flex-1" />
      <div className="flex items-end justify-between gap-6">
        <div className="min-w-0">
          <h1 className="display truncate text-[44px] leading-none tracking-[-0.02em]">{title}</h1>
          <div className="mt-3 text-sm text-white/80">{sub}</div>
          {r.warning && r.state !== "error" && <p className="mt-2 text-xs text-white/60">{r.warning}</p>}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {r.state === "error" ? (
            <Button variant="primary" onClick={() => api.startBuffer()}>
              <ArrowClockwise size={16} />
              Try again
            </Button>
          ) : live || r.state === "starting" ? (
            <>
              <Button variant="ghost" className={ghostOnPhoto} onClick={() => api.stopBuffer()}>
                <Stop size={14} weight="fill" />
                Stop
              </Button>
              <Button variant="primary" disabled={!r.canSave || r.saving} onClick={() => api.saveClip()}>
                {r.saving && <CircleNotch size={16} className="animate-spin" />}
                Save clip
              </Button>
            </>
          ) : (
            <>
              {r.canSave && (
                <Button variant="ghost" className={ghostOnPhoto} onClick={() => api.saveClip()}>
                  Save what's there
                </Button>
              )}
              <Button variant="primary" onClick={() => api.startBuffer()}>
                Start capturing
              </Button>
            </>
          )}
        </div>
      </div>
    </Banner>
  );
}
