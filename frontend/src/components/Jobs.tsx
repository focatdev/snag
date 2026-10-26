import { CheckCircle, Copy, FolderOpen, WarningCircle, X } from "@phosphor-icons/react";
import { api, type Job } from "@/lib/api";
import { bytes, cn } from "@/lib/format";
import { useStore } from "@/lib/store";
import { Button } from "./ui/button";

function doneText(j: Job) {
  if (j.kind === "upload") return j.output.replace("https://", "");
  if (j.kind === "shrink") return j.size > 0 && j.size < j.origSize * 0.9 ? `${bytes(j.origSize - j.size)} smaller` : "Already small, kept as is";
  if (j.replace) return j.origSize > j.size ? `Replaced, ${bytes(j.origSize - j.size)} smaller` : "Replaced original";
  return bytes(j.size);
}

export function JobsPanel() {
  const jobs = useStore((s) => s.jobs).filter((j) => j.kind !== "preview");
  const clips = useStore((s) => s.clips);
  return (
    <div>
      <div className="flex items-center justify-between border-b border-line px-4 py-2.5">
        <span className="text-sm font-medium">Activity</span>
        {jobs.some((j) => j.status !== "running" && j.status !== "queued") && (
          <button className="text-xs text-muted hover:text-fg" onClick={() => api.clearJobs()}>
            Clear finished
          </button>
        )}
      </div>
      {jobs.length === 0 ? (
        <p className="px-4 py-6 text-center text-sm text-muted">Exports, uploads, merges and shrinks show up here</p>
      ) : (
        <ul className="max-h-[320px] overflow-y-auto py-1">
          {jobs.map((j) => {
            const clip = clips.find((c) => c.id === j.clipId);
            return (
              <li key={j.id} className="group flex items-center gap-3 px-4 py-2">
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline gap-2">
                    <span className="truncate text-sm text-fg">{j.kind === "merge" ? "Montage" : (clip?.title ?? "Clip")}</span>
                    <span className="shrink-0 text-2xs text-muted">{j.label}</span>
                  </div>
                  {j.status === "running" || j.status === "queued" ? (
                    <div className="mt-1.5 h-[3px] overflow-hidden rounded-full bg-line">
                      <div className="h-full bg-frost transition-[width] duration-200" style={{ width: `${Math.round(j.progress * 100)}%` }} />
                    </div>
                  ) : (
                    <div className={cn("mt-0.5 flex items-center gap-1 text-2xs", j.status === "failed" ? "text-bad" : "text-muted")}>
                      {j.status === "done" && <CheckCircle size={12} weight="fill" className="text-ok" />}
                      {j.status === "failed" && <WarningCircle size={12} weight="fill" />}
                      <span className="truncate" title={j.error}>
                        {j.status === "done" ? doneText(j) : j.status === "failed" ? j.error : "Canceled"}
                      </span>
                    </div>
                  )}
                </div>
                {j.status === "done" && j.kind !== "shrink" && !j.replace && (
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={j.kind === "upload" ? "Copy link" : "Copy to clipboard"}
                    onClick={() =>
                      (j.kind === "upload" ? api.copyText(j.output) : api.copyPath(j.output)).then(() =>
                        useStore.getState().toast("success", j.kind === "upload" ? "Link copied" : "Copied. Paste it anywhere with Ctrl+V"),
                      )
                    }
                  >
                    <Copy size={15} />
                  </Button>
                )}
                {j.status === "done" && j.kind !== "upload" && (
                  <Button size="icon-sm" variant="ghost" aria-label="Show in folder" onClick={() => (j.kind === "shrink" || j.replace ? api.revealClip(j.clipId) : api.revealPath(j.output))}>
                    <FolderOpen size={15} />
                  </Button>
                )}
                {(j.status === "running" || j.status === "queued") && (
                  <Button size="icon-sm" variant="ghost" aria-label="Cancel" onClick={() => api.cancelJob(j.id)}>
                    <X size={14} />
                  </Button>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
