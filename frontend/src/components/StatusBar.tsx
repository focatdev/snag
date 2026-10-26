import * as P from "@radix-ui/react-popover";
import { CircleNotch } from "@phosphor-icons/react";
import type { ReactNode } from "react";
import { bytes, cn } from "@/lib/format";
import { useStore } from "@/lib/store";
import { JobsPanel } from "./Jobs";

function Seg({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("flex h-full items-center gap-1.5 border-r border-line-soft px-3 last:border-r-0", className)}>{children}</div>;
}

export function StatusBar() {
  const st = useStore((s) => s.status);
  const jobs = useStore((s) => s.jobs);
  if (!st) return <footer className="h-7 shrink-0 border-t border-line-soft" />;
  const r = st.recorder;
  const running = jobs.filter((j) => j.kind !== "preview" && (j.status === "running" || j.status === "queued"));

  // sidebar shows normal state, this is only for problems
  let state: ReactNode = null;
  if (!st.ffmpegReady) state = <span className="text-warn">Needs FFmpeg</span>;
  else if (r.state === "starting")
    state = (
      <>
        <CircleNotch size={12} className="animate-spin" />
        <span className="text-fg">Starting</span>
      </>
    );
  else if (r.state === "error") state = <span className="truncate text-bad" title={r.error}>Capture stopped: {r.error}</span>;
  else if (r.warning) state = <span className="truncate text-warn" title={r.warning}>{r.warning}</span>;

  return (
    <footer className="flex h-7 shrink-0 items-center border-t border-line-soft bg-canvas text-2xs text-muted">
      {state && <Seg className="max-w-[520px]">{state}</Seg>}
      {r.state === "buffering" && (
        <>
          <Seg>{r.encoderLabel || r.encoder}</Seg>
          <Seg>
            {r.width}x{r.height} at {r.fps} fps
          </Seg>
          <Seg>{(r.kbps / 1000).toFixed(1)} Mbps</Seg>
        </>
      )}
      <div className="flex-1" />
      <Seg>CPU {r.cpu.toFixed(1)}%</Seg>
      <Seg>RAM {bytes(r.memBytes, 0)}</Seg>
      <P.Root>
        <P.Trigger asChild>
          <button className="flex h-full items-center gap-1.5 px-3 hover:bg-hover hover:text-fg">
            {running.length > 0 ? (
              <>
                <CircleNotch size={12} className="animate-spin text-frost" />
                <span className="text-fg">
                  {running.length === 1 ? (running[0].kind === "shrink" ? "Shrinking" : running[0].kind === "merge" ? "Merging" : running[0].kind === "upload" ? "Uploading" : "Exporting") : `${running.length} running`}
                </span>
              </>
            ) : (
              <span>Activity</span>
            )}
          </button>
        </P.Trigger>
        <P.Portal>
          <P.Content side="top" align="end" sideOffset={6} className="anim-pop z-50 w-[360px] rounded-card border border-line bg-raised shadow-[0_16px_40px_rgba(0,0,0,0.5)]">
            <JobsPanel />
          </P.Content>
        </P.Portal>
      </P.Root>
    </footer>
  );
}
