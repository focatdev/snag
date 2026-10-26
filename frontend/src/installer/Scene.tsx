import { useLayoutEffect, useRef, useState } from "react";
import { HOOK } from "@/components/Logo";
import { cn } from "@/lib/format";

// hook eye position in the 1120 unit svg
const EYE_X = 666;
const EYE_TOP = 92;
const BOWL_X = 560;
const BOWL_BOTTOM = 1060;
const SCALE = 0.1;

export function Scene({ images, active, depth, draw, caught, children }: { images: string[]; active: number; depth: number; draw: boolean; caught: boolean; children?: React.ReactNode }) {
  const path = useRef<SVGPathElement>(null);
  const [len, setLen] = useState(0);
  useLayoutEffect(() => {
    if (path.current) setLen(path.current.getTotalLength());
  }, []);
  const w = 1120 * SCALE;

  return (
    <div className="drag relative h-full overflow-hidden bg-black">
      {images.map((src, i) => (
        <img
          key={src}
          src={src}
          alt=""
          className="absolute inset-0 h-full w-full object-cover transition-opacity duration-[900ms] ease-out"
          style={{ opacity: i === active ? 0.55 : 0 }}
        />
      ))}

      <div className="scene-sway absolute left-1/2 top-0" style={{ transformOrigin: "0 0" }}>
        <div className={cn("relative transition-[height] duration-[1100ms] ease-[cubic-bezier(.34,1.3,.5,1)]", caught && "scene-snag")} style={{ height: depth }}>
          <div className="absolute left-0 top-0 h-full w-px -translate-x-1/2 bg-white/70" />
          <div className="absolute top-full" style={{ left: -EYE_X * SCALE, marginTop: -EYE_TOP * SCALE, width: w }}>
            <svg viewBox="0 0 1120 1120" width={w} height={w} className="overflow-visible drop-shadow-[0_6px_18px_rgba(0,0,0,0.45)]">
              <path
                ref={path}
                d={HOOK}
                fillRule="evenodd"
                className={draw ? "hook-draw" : undefined}
                style={{ ["--len" as string]: len, fill: "white", stroke: "white", strokeWidth: 14 } as React.CSSProperties}
              />
              <circle cx="683" cy="752" r="40" fill={caught ? "var(--color-live)" : "white"} className="transition-colors duration-500" />
            </svg>
            {caught && (
              <div className="frame-catch absolute" style={{ left: BOWL_X * SCALE - 30, top: BOWL_BOTTOM * SCALE - 6 }}>
                <svg width="60" height="46" viewBox="0 0 60 46" aria-hidden>
                  <rect x="1" y="1" width="58" height="44" rx="6" fill="#eef2f7" />
                  {[0, 1, 2, 3, 4].map((i) => (
                    <rect key={`t${i}`} x={7 + i * 10.5} y="5" width="5" height="4" rx="1" fill="#151a21" opacity="0.8" />
                  ))}
                  {[0, 1, 2, 3, 4].map((i) => (
                    <rect key={`b${i}`} x={7 + i * 10.5} y="37" width="5" height="4" rx="1" fill="#151a21" opacity="0.8" />
                  ))}
                  <rect x="7" y="13" width="46" height="20" rx="2" fill="#151a21" opacity="0.85" />
                  <path d="M26 18.5v9l8-4.5z" fill="#eef2f7" />
                </svg>
              </div>
            )}
          </div>
        </div>
      </div>
      {children}
    </div>
  );
}
