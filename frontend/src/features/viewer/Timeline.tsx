import { SpeakerHigh, SpeakerSlash, X } from "@phosphor-icons/react";
import * as P from "@radix-ui/react-popover";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import frost from "@/assets/live.jpg";
import { Slider } from "@/components/ui/slider";
import { api, type Clip, type TrackMix } from "@/lib/api";
import { clock, cn } from "@/lib/format";
import { mediaUrl, useStore } from "@/lib/store";

const HEAD = 124;

function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [w, setW] = useState(0);
  useLayoutEffect(() => {
    if (!ref.current) return;
    const ro = new ResizeObserver(([e]) => setW(e.contentRect.width));
    ro.observe(ref.current);
    return () => ro.disconnect();
  }, []);
  return [ref, w] as const;
}

function Wave({ data, inOut, duration, muted }: { data: number[] | null; inOut: [number, number]; duration: number; muted: boolean }) {
  const [ref, w] = useWidth<HTMLDivElement>();
  const canvas = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const c = canvas.current;
    if (!c || !w) return;
    const dpr = window.devicePixelRatio || 1;
    const h = 36;
    c.width = Math.floor(w * dpr);
    c.height = Math.floor(h * dpr);
    const ctx = c.getContext("2d")!;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, w, h);
    if (!data || data.length === 0) {
      ctx.fillStyle = "#2a2d31";
      ctx.fillRect(0, h / 2, w, 1);
      return;
    }
    const bar = 2;
    const gap = 1;
    const n = Math.floor(w / (bar + gap));
    const a = (inOut[0] / duration) * w;
    const b = (inOut[1] / duration) * w;
    for (let i = 0; i < n; i++) {
      const x = i * (bar + gap);
      const v = data[Math.floor((i / n) * data.length)] ?? 0;
      const bh = Math.max(1, v * (h - 4));
      const inside = x >= a && x <= b;
      ctx.fillStyle = muted ? "#34383e" : inside ? "#b8c3d1" : "#4a5059";
      ctx.fillRect(x, (h - bh) / 2, bar, bh);
    }
  }, [data, w, inOut, duration, muted]);
  return (
    <div ref={ref} className="h-9 w-full">
      <canvas ref={canvas} style={{ width: w, height: 36 }} />
    </div>
  );
}

function ticks(duration: number, width: number) {
  const steps = [1, 2, 5, 10, 15, 30, 60, 120];
  const step = steps.find((s) => (s / duration) * width >= 64) ?? 120;
  const out: number[] = [];
  for (let t = 0; t <= duration + 0.001; t += step) out.push(t);
  return out;
}

export function Timeline({
  clip,
  duration,
  time,
  inOut,
  setInOut,
  onSeek,
  mix,
  setMix,
  cuts = [],
  onRemoveCut,
}: {
  clip: Clip;
  duration: number;
  time: number;
  inOut: [number, number];
  setInOut: (v: [number, number]) => void;
  onSeek: (t: number) => void;
  mix: TrackMix[];
  setMix: (m: TrackMix[]) => void;
  cuts?: [number, number][];
  onRemoveCut?: (i: number) => void;
}) {
  const [lane, w] = useWidth<HTMLDivElement>();
  const [waves, setWaves] = useState<(number[] | null)[]>([]);
  const drag = useRef<"in" | "out" | "seek" | null>(null);
  const mock = !useStore.getState().boot?.media.base;
  const tracks = clip.tracks ?? [];
  const frames = clip.spriteFrames || 10;
  const sprite = clip.sprite ? mediaUrl("sprite", clip.id, clip.size) : "";

  useEffect(() => {
    let alive = true;
    Promise.all(tracks.map((_, i) => api.waveform(clip.id, i, 1200).catch(() => null))).then((w) => alive && setWaves(w));
    return () => {
      alive = false;
    };
  }, [clip.id, tracks.length]);

  const px = (t: number) => (duration > 0 ? (t / duration) * w : 0);
  const at = (clientX: number) => {
    const r = lane.current!.getBoundingClientRect();
    return Math.max(0, Math.min(duration, ((clientX - r.left) / r.width) * duration));
  };

  const down = (e: React.PointerEvent) => {
    if (!duration) return;
    const t = at(e.clientX);
    const near = (v: number) => Math.abs(px(v) - px(t)) < 9;
    drag.current = near(inOut[0]) ? "in" : near(inOut[1]) ? "out" : "seek";
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
    move(e);
  };
  const move = (e: React.PointerEvent) => {
    if (!drag.current) return;
    const t = at(e.clientX);
    const minLen = Math.min(0.5, duration);
    if (drag.current === "in") setInOut([Math.min(t, inOut[1] - minLen), inOut[1]]);
    else if (drag.current === "out") setInOut([inOut[0], Math.max(t, inOut[0] + minLen)]);
    else onSeek(t);
  };
  const up = () => (drag.current = null);

  const setTrack = (i: number, p: Partial<TrackMix>) => setMix(mix.map((m) => (m.index === i ? { ...m, ...p } : m)));
  const rows = 22 + 48 + tracks.length * 40;

  return (
    <div className="select-none rounded-card border border-line-soft bg-raised">
      <div className="flex">
        <div className="shrink-0 border-r border-line-soft" style={{ width: HEAD }}>
          <div className="h-[22px] border-b border-line-soft" />
          <div className="flex h-12 items-center border-b border-line-soft px-3 text-xs text-muted">Video</div>
          {tracks.map((name, i) => {
            const m = mix.find((x) => x.index === i) ?? { index: i, volume: 1, muted: false };
            return (
              <div key={i} className="flex h-10 items-center gap-1 border-b border-line-soft px-2 last:border-b-0">
                <button
                  aria-label={m.muted ? `Unmute ${name}` : `Mute ${name}`}
                  onClick={() => setTrack(i, { muted: !m.muted })}
                  className={cn("grid h-6 w-6 place-items-center rounded-[4px] hover:bg-hover", m.muted ? "text-faint" : "text-fg")}
                >
                  {m.muted ? <SpeakerSlash size={14} /> : <SpeakerHigh size={14} />}
                </button>
                <P.Root>
                  <P.Trigger asChild>
                    <button className={cn("min-w-0 flex-1 truncate rounded-[4px] px-1 py-0.5 text-left text-xs hover:bg-hover", m.muted ? "text-faint line-through" : "text-fg")}>
                      {name}
                      {Math.abs(m.volume - 1) > 0.01 && <span className="ml-1 text-faint">{Math.round(m.volume * 100)}%</span>}
                    </button>
                  </P.Trigger>
                  <P.Portal>
                    <P.Content side="right" sideOffset={8} className="anim-pop z-50 w-[220px] rounded-[8px] border border-line bg-raised p-3 shadow-[0_12px_32px_rgba(0,0,0,0.45)]">
                      <div className="mb-2 flex justify-between text-xs">
                        <span className="text-muted">{name} volume in export</span>
                        <span>{Math.round(m.volume * 100)}%</span>
                      </div>
                      <Slider min={0} max={2} step={0.05} value={[m.volume]} onValueChange={([v]) => setTrack(i, { volume: v })} />
                    </P.Content>
                  </P.Portal>
                </P.Root>
              </div>
            );
          })}
        </div>

        <div className="min-w-0 flex-1 px-3">
        <div ref={lane} className="relative cursor-text" onPointerDown={down} onPointerMove={move} onPointerUp={up}>
          <div className="relative h-[22px] border-b border-line-soft">
            {w > 0 &&
              ticks(duration, w)
                .filter((t) => px(t) < w - 28)
                .map((t) => (
                <div key={t} className="absolute top-0 h-full" style={{ left: px(t) }}>
                  <div className="h-1.5 w-px bg-line" />
                  <span className="absolute left-1 top-[5px] text-[10px] leading-none text-faint">{clock(t)}</span>
                </div>
              ))}
            {w > 0 &&
              cuts.map(([a, b], i) => (
                <button
                  key={`cut${i}`}
                  aria-label="Put this part back"
                  title="Put this part back"
                  onPointerDown={(e) => e.stopPropagation()}
                  onClick={() => onRemoveCut?.(i)}
                  className="absolute top-[3px] z-10 grid h-4 w-4 -translate-x-1/2 place-items-center rounded-full bg-bad text-frost-ink hover:bg-[#f5a29b]"
                  style={{ left: px((a + b) / 2) }}
                >
                  <X size={9} weight="bold" />
                </button>
              ))}
            {(clip.markers ?? []).map((m, i) => (
              <div key={i} className="absolute top-0 -translate-x-1/2" style={{ left: px(m) }} title={`Bookmark at ${clock(m, true)}`}>
                <svg width="10" height="8" viewBox="0 0 10 8">
                  <path d="M0 0h10L5 8z" fill="var(--color-warn)" />
                </svg>
              </div>
            ))}
          </div>

          <div className="flex h-12 gap-px overflow-hidden border-b border-line-soft bg-well py-1">
            {Array.from({ length: frames }).map((_, i) => (
              <div
                key={i}
                className="h-full flex-1 bg-cover"
                style={
                  sprite
                    ? { backgroundImage: `url(${sprite})`, backgroundSize: `${frames * 100}% 100%`, backgroundPosition: `${(i / (frames - 1)) * 100}% 0` }
                    : mock
                      ? { backgroundImage: `url(${frost})`, backgroundSize: "400%", backgroundPosition: `${i * 11}% ${30 + i * 4}%`, filter: "brightness(0.8)" }
                      : undefined
                }
              />
            ))}
          </div>

          {tracks.map((_, i) => {
            const m = mix.find((x) => x.index === i);
            return (
              <div key={i} className="flex h-10 items-center border-b border-line-soft last:border-b-0">
                <Wave data={waves[i] ?? null} inOut={inOut} duration={duration || 1} muted={!!m?.muted} />
              </div>
            );
          })}

          {w > 0 && duration > 0 && (
            <div className="pointer-events-none absolute inset-0" style={{ height: rows }}>
              <div className="absolute inset-y-0 left-0 bg-canvas/70" style={{ width: px(inOut[0]) }} />
              <div className="absolute inset-y-0 right-0 bg-canvas/70" style={{ width: w - px(inOut[1]) }} />
              {cuts.map(([a, b], i) => (
                <div
                  key={i}
                  className="absolute inset-y-0 border-x border-bad/60 bg-[repeating-linear-gradient(135deg,rgba(240,133,124,0.28)_0_2px,transparent_2px_7px)]"
                  style={{ left: px(a), width: Math.max(2, px(b) - px(a)) }}
                />
              ))}
              <div className="absolute inset-y-0 border-y-2 border-frost/80" style={{ left: px(inOut[0]), width: px(inOut[1]) - px(inOut[0]) }} />
              {[inOut[0], inOut[1]].map((t, i) => (
                <div key={i} className={cn("absolute inset-y-0 flex w-2.5 items-center justify-center bg-frost", i === 0 ? "-translate-x-full rounded-l-[4px]" : "rounded-r-[4px]")} style={{ left: px(t) }}>
                  <span className="h-4 w-px bg-frost-ink/40" />
                </div>
              ))}
              <div className="absolute inset-y-0 w-px bg-white" style={{ left: px(time) }}>
                <span className="absolute -left-[4px] -top-px h-0 w-0 border-x-[4.5px] border-t-[6px] border-x-transparent border-t-white" />
              </div>
            </div>
          )}
        </div>
        </div>
      </div>
    </div>
  );
}
