import { ArrowLeft, ArrowLineLeft, ArrowLineRight, ArrowsInSimple, CaretLeft, Keyboard, CircleNotch, CloudArrowUp, LinkSimple, Copy, CaretRight, DotsThree, FolderOpen, FolderSimple, Microphone, Pause, Play, Repeat, Scissors, Sparkle, SpeakerHigh, SpeakerSlash, Star, TextAlignLeft, Trash } from "@phosphor-icons/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import frost from "@/assets/live.jpg";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Kbd } from "@/components/ui/kbd";
import { DropMenu } from "@/components/ui/menu";
import { Slider } from "@/components/ui/slider";
import { Tip } from "@/components/ui/tooltip";
import { aiReady, copyLink, nameWithAI, shrink, transcribe, uploadClip } from "@/lib/actions";
import { api, type TrackMix } from "@/lib/api";
import { mockThumb } from "@/lib/mock";
import { bytes, cn, fullDate, resLabel, timecode } from "@/lib/format";
import { mediaUrl, useStore } from "@/lib/store";
import { copyClip } from "@/components/ClipCard";
import { folderMenu } from "@/features/library/Folders";
import { ExportPanel, defaultDraft, needsEncode, type Draft } from "./ExportPanel";
import { Timeline } from "./Timeline";

function CropOverlay({ video, cropX, onChange }: { video: HTMLVideoElement | null; cropX: number; onChange: (v: number) => void }) {
  const [box, setBox] = useState({ x: 0, y: 0, w: 0, h: 0 });
  const dragging = useRef<{ start: number; from: number } | null>(null);
  useEffect(() => {
    if (!video) return;
    const measure = () => {
      const vw = video.videoWidth || 16;
      const vh = video.videoHeight || 9;
      const cw = video.clientWidth;
      const ch = video.clientHeight;
      const scale = Math.min(cw / vw, ch / vh);
      const w = vw * scale;
      const h = vh * scale;
      setBox({ x: (cw - w) / 2, y: (ch - h) / 2, w, h });
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(video);
    video.addEventListener("loadedmetadata", measure);
    return () => {
      ro.disconnect();
      video.removeEventListener("loadedmetadata", measure);
    };
  }, [video]);
  const cw = (box.h * 9) / 16;
  const left = box.x + (box.w - cw) * cropX;
  return (
    <div className="absolute inset-0">
      <div className="absolute bg-black/65" style={{ left: box.x, top: box.y, width: left - box.x, height: box.h }} />
      <div className="absolute bg-black/65" style={{ left: left + cw, top: box.y, width: box.x + box.w - left - cw, height: box.h }} />
      <div
        className="absolute cursor-ew-resize border-2 border-frost"
        style={{ left, top: box.y, width: cw, height: box.h }}
        onPointerDown={(e) => {
          (e.target as HTMLElement).setPointerCapture(e.pointerId);
          dragging.current = { start: e.clientX, from: cropX };
        }}
        onPointerMove={(e) => {
          if (!dragging.current || box.w <= cw) return;
          const d = (e.clientX - dragging.current.start) / (box.w - cw);
          onChange(Math.max(0, Math.min(1, dragging.current.from + d)));
        }}
        onPointerUp={() => (dragging.current = null)}
      />
    </div>
  );
}

export function Viewer({ id }: { id: string }) {
  const clip = useStore((s) => s.clips.find((c) => c.id === id));
  const go = useStore((s) => s.go);
  const patch = useStore((s) => s.patchClip);
  const toast = useStore((s) => s.toast);
  const jobs = useStore((s) => s.jobs);
  const folders = useStore((s) => s.folders);
  const video = useRef<HTMLVideoElement>(null);
  const [videoEl, setVideoEl] = useState<HTMLVideoElement | null>(null);
  const [time, setTime] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [duration, setDuration] = useState(clip?.duration ?? 0);
  const [inOut, setInOut] = useState<[number, number]>([0, clip?.duration ?? 0]);
  const [loop, setLoop] = useState(true);
  const [volume, setVolume] = useState(0.8);
  const [muted, setMuted] = useState(false);
  const [mix, setMix] = useState<TrackMix[]>((clip?.tracks ?? []).map((_, i) => ({ index: i, volume: 1, muted: false })));
  const [draft, setDraft] = useState<Draft>(defaultDraft);
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(clip?.title ?? "");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [previewId, setPreviewId] = useState<string | null>(null);
  const [lastExport, setLastExport] = useState<string | null>(null);
  const [cuts, setCuts] = useState<[number, number][]>([]);
  const [showText, setShowText] = useState(false);
  const [naming, setNaming] = useState(false);
  const settings = useStore((s) => s.settings);
  const mock = !useStore.getState().boot?.media.base;

  const fps = clip?.fps || 60;
  const lastJob = lastExport ? jobs.find((j) => j.id === lastExport) : undefined;
  // release the file so the backend can replace it
  const detached = !!lastJob?.replace && (lastJob.status === "running" || lastJob.status === "queued");
  const [repairing, setRepairing] = useState(false);
  const [reloads, setReloads] = useState(0);
  const repairTried = useRef(false);
  const resumeAt = useRef<number | null>(null);
  const base = clip && !detached && !repairing ? mediaUrl("clip", clip.id, clip.size) : "";
  const src = base && reloads ? `${base}&r=${reloads}` : base;

  useEffect(() => {
    if (clip && clip.duration && duration === 0) {
      setDuration(clip.duration);
      setInOut([0, clip.duration]);
    }
  }, [clip, duration]);

  const seek = useCallback((t: number) => {
    const v = video.current;
    setTime(t);
    if (v) v.currentTime = t;
  }, []);

  const toggle = useCallback(() => {
    const v = video.current;
    if (!v) {
      setPlaying((p) => !p);
      return;
    }
    if (v.paused) {
      if (v.ended || v.currentTime >= inOut[1] - 0.05 || v.currentTime < inOut[0]) v.currentTime = inOut[0];
      v.play();
    } else v.pause();
  }, [inOut]);

  const cutOut = useCallback(() => {
    if (inOut[0] <= 0.01 && inOut[1] >= duration - 0.01) {
      toast("info", "Mark the part to remove with Set start and Set end first");
      return;
    }
    setCuts((c) => [...c, inOut].sort((a, b) => a[0] - b[0]));
    setInOut([0, duration]);
  }, [inOut, duration, toast]);

  // <video> can't loop a sub-range
  useEffect(() => {
    let raf = 0;
    const tick = () => {
      const v = video.current;
      if (v) {
        setTime(v.currentTime);
        const cut = !v.paused && cuts.find((c) => v.currentTime >= c[0] && v.currentTime < c[1] - 0.02);
        if (cut) v.currentTime = cut[1];
        if (!v.paused && v.currentTime >= inOut[1]) {
          if (loop) v.currentTime = inOut[0];
          else v.pause();
        }
      } else if (mock && playing) {
        setTime((t) => (t + 1 / 60 > inOut[1] ? inOut[0] : t + 1 / 60));
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [inOut, loop, mock, playing, cuts]);

  useEffect(() => {
    if (video.current) {
      video.current.volume = volume;
      video.current.muted = muted;
    }
  }, [volume, muted]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest("input, textarea, [role=slider]")) return;
      const step = 1 / fps;
      switch (e.key) {
        case " ":
        case "k":
          e.preventDefault();
          toggle();
          break;
        case "ArrowLeft":
        case "j":
          seek(Math.max(0, time - (e.shiftKey ? 5 : 1)));
          break;
        case "ArrowRight":
        case "l":
          seek(Math.min(duration, time + (e.shiftKey ? 5 : 1)));
          break;
        case ",":
          video.current?.pause();
          seek(Math.max(0, time - step));
          break;
        case ".":
          video.current?.pause();
          seek(Math.min(duration, time + step));
          break;
        case "i":
          setInOut([Math.min(time, inOut[1] - 0.5), inOut[1]]);
          break;
        case "o":
          setInOut([inOut[0], Math.max(time, inOut[0] + 0.5)]);
          break;
        case "m":
          setMuted((m) => !m);
          break;
        case "x":
          cutOut();
          break;
        case "Escape":
          if (!editing) go({ name: "library" });
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toggle, seek, time, duration, fps, inOut, editing, go, cutOut]);

  useEffect(() => {
    if (!previewId) return;
    const j = jobs.find((x) => x.id === previewId);
    if (j?.status === "failed") {
      toast("error", j.error || "Preview failed");
      setPreviewId(null);
    }
  }, [jobs, previewId, toast]);

  const volumesChanged = mix.some((m) => !m.muted && Math.abs(m.volume - 1) > 0.01);
  const mixed = draft.mixAudio && mix.filter((m) => !m.muted).length > 1;
  const encode = needsEncode(draft, mixed, volumesChanged, cuts.some((c) => c[1] > inOut[0] && c[0] < inOut[1]));
  const removed = cuts.reduce((a, c) => a + Math.max(0, Math.min(c[1], inOut[1]) - Math.max(c[0], inOut[0])), 0);
  const kept = Math.max(0, inOut[1] - inOut[0] - removed);
  const previewJob = previewId ? jobs.find((j) => j.id === previewId) : undefined;

  const submit = async (preview: boolean) => {
    if (!clip) return;
    try {
      const j = await api.exportClip({ ...draft, fast: !encode, clipId: clip.id, start: inOut[0], end: inOut[1], tracks: mix, preview, name: "", cuts, replaceOriginal: draft.replaceOriginal && !preview });
      if (preview) setPreviewId(j.id);
      else {
        setLastExport(j.id);
        if (draft.replaceOriginal) setCuts([]);
      }
    } catch (e) {
      toast("error", String(e));
    }
  };

  const info = useMemo(
    () =>
      clip
        ? [
            ["Resolution", `${resLabel(clip.height)}${Math.round(clip.fps)}`],
            ["Codec", clip.codec.toUpperCase()],
            ["Size", bytes(clip.size)],
            ["Recorded", fullDate(clip.createdAt)],
          ]
        : [],
    [clip],
  );

  if (!clip) {
    return (
      <div className="grid h-full place-items-center text-center">
        <div>
          <p className="text-base font-medium">This clip is gone</p>
          <p className="mt-1 text-sm text-muted">It was deleted or moved out of the clips folder.</p>
          <Button className="mt-4" onClick={() => go({ name: "library" })}>
            Back to library
          </Button>
        </div>
      </div>
    );
  }

  const commitTitle = async () => {
    setEditing(false);
    if (title.trim() && title !== clip.title) patch(await api.updateClip(clip.id, { title: title.trim() }));
    else setTitle(clip.title);
  };

  return (
    <div className="flex h-full">
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex shrink-0 items-center gap-2 border-b border-line-soft px-4 py-2">
          <Button variant="ghost" size="icon-sm" aria-label="Back to library" onClick={() => go({ name: "library" })}>
            <ArrowLeft size={16} />
          </Button>
          {editing ? (
            <input
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onBlur={commitTitle}
              onKeyDown={(e) => {
                if (e.key === "Enter") commitTitle();
                if (e.key === "Escape") {
                  setTitle(clip.title);
                  setEditing(false);
                }
              }}
              className="h-7 w-[380px] rounded-[5px] border border-[#4a5260] bg-well px-2 text-base font-medium outline-none"
            />
          ) : (
            <button onClick={() => setEditing(true)} className="truncate rounded-[5px] px-2 py-0.5 text-base font-medium hover:bg-hover" title="Rename">
              {clip.title}
            </button>
          )}
          <span className="shrink-0 text-sm text-muted">{clip.game}</span>
          <div className="flex-1" />
          <div className="mr-2 flex items-center gap-3 text-xs text-faint">
            {info.map(([k, v]) => (
              <span key={k} title={k}>
                {v}
              </span>
            ))}
          </div>
          {aiReady() && !clip.export && (
            <Tip label={clip.aiTitled ? "Name it again with AI" : "Name with AI"}>
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={naming}
                onClick={async () => {
                  setNaming(true);
                  await nameWithAI(clip.id);
                  setNaming(false);
                }}
              >
                <Sparkle size={16} weight={clip.aiTitled ? "fill" : "regular"} className={naming ? "animate-pulse" : ""} />
              </Button>
            </Tip>
          )}
          <Tip label={clip.favorite ? "Remove favorite" : "Favorite"}>
            <Button variant="ghost" size="icon-sm" onClick={async () => patch(await api.updateClip(clip.id, { favorite: !clip.favorite }))}>
              <Star size={16} weight={clip.favorite ? "fill" : "regular"} className={clip.favorite ? "text-warn" : ""} />
            </Button>
          </Tip>
          {clip.link ? (
            <Tip label="Copy link">
              <Button variant="ghost" size="icon-sm" onClick={() => copyLink(clip.link!)}>
                <LinkSimple size={16} />
              </Button>
            </Tip>
          ) : (
            <Tip label="Upload and copy link">
              <Button variant="ghost" size="icon-sm" onClick={() => uploadClip(clip.id)}>
                <CloudArrowUp size={16} />
              </Button>
            </Tip>
          )}
          <Tip label="Copy file, then paste into Discord">
            <Button variant="ghost" size="icon-sm" onClick={() => copyClip(clip.id)}>
              <Copy size={16} />
            </Button>
          </Tip>
          <Tip label="Show in folder">
            <Button variant="ghost" size="icon-sm" onClick={() => api.revealClip(clip.id)}>
              <FolderOpen size={16} />
            </Button>
          </Tip>
          <DropMenu
            trigger={
              <Button variant="ghost" size="icon-sm" aria-label="More">
                <DotsThree size={18} weight="bold" />
              </Button>
            }
            items={[
              { label: "Move to folder", icon: <FolderSimple size={15} />, sub: folderMenu(folders, [clip.id], [clip.folder]) },
              { label: "Upload and copy link", icon: <CloudArrowUp size={15} />, onSelect: () => uploadClip(clip.id) },
              ...(clip.link ? [{ label: "Copy link", icon: <LinkSimple size={15} />, onSelect: () => copyLink(clip.link!) }] : []),
        ...(clip.deleteLink ? [{ label: "Copy delete link", icon: <LinkSimple size={15} />, onSelect: () => copyLink(clip.deleteLink!) }] : []),
              { label: clip.shrunk ? "Already shrunk" : "Shrink file", icon: <ArrowsInSimple size={15} />, disabled: clip.shrunk, onSelect: () => shrink([clip.id]) },
              ...(aiReady() && settings?.aiProvider !== "anthropic" && (clip.tracks?.length ?? 0) > 0
                ? [{ label: clip.transcript ? "Transcribe again" : "Transcribe voice", icon: <Microphone size={15} />, onSelect: () => transcribe(clip.id) }]
                : []),
              ...(clip.transcript ? [{ label: "What was said", icon: <TextAlignLeft size={15} />, onSelect: () => setShowText(true) }] : []),
              "sep" as const,
              { label: "Move to recycle bin", icon: <Trash size={15} />, danger: true, onSelect: () => setConfirmDelete(true) },
            ]}
          />
        </div>

        <div className="relative min-h-0 flex-1 bg-black">
          {mock ? (
            <div className="absolute inset-0 m-auto aspect-video max-h-full max-w-full" style={{ backgroundImage: `url(${mockThumb(clip) ?? frost})`, backgroundSize: "cover", backgroundPosition: `${30 + (time / (duration || 1)) * 40}% 50%` }} />
          ) : (
            <video
              ref={(el) => {
                video.current = el;
                setVideoEl(el);
              }}
              src={src}
              className="absolute inset-0 h-full w-full object-contain"
              preload="auto"
              onLoadedMetadata={(e) => {
                const v = e.currentTarget;
                const d = v.duration;
                if (isFinite(d) && d > 0 && Math.abs(d - duration) > 0.05) {
                  setDuration(d);
                  setInOut([0, d]);
                }
                if (resumeAt.current !== null) {
                  v.currentTime = Math.min(resumeAt.current, Math.max(0, d - 0.1));
                  resumeAt.current = null;
                }
              }}
              onEnded={(e) => {
                const v = e.currentTarget;
                if (loop) {
                  v.currentTime = inOut[0];
                  v.play();
                }
              }}
              // a decode error kills the element, older clips can have a broken last frame
              onError={(e) => {
                const err = e.currentTarget.error;
                if (!err || !clip) return;
                resumeAt.current = Math.max(0, time - 0.5);
                if (err.code === 3 && !repairTried.current) {
                  repairTried.current = true;
                  setRepairing(true);
                  api
                    .repairClip(clip.id)
                    .then((c) => {
                      patch(c);
                      toast("success", "Fixed a cut-off frame at the end of this clip");
                    })
                    .catch((x) => toast("error", String(x)))
                    .finally(() => {
                      setRepairing(false);
                      setReloads((n) => n + 1);
                    });
                  return;
                }
                if (reloads < 3) setReloads((n) => n + 1);
                else toast("error", "The player can't decode part of this clip");
              }}
              onPlay={() => setPlaying(true)}
              onPause={() => setPlaying(false)}
              onClick={toggle}
            />
          )}
          {repairing && (
            <div className="absolute inset-0 grid place-items-center bg-black/60 text-sm text-fg">
              <span className="flex items-center gap-2">
                <CircleNotch size={16} className="animate-spin" />
                Fixing the end of this clip
              </span>
            </div>
          )}
          {draft.vertical && <CropOverlay video={videoEl} cropX={draft.cropX} onChange={(v) => setDraft({ ...draft, cropX: v })} />}
        </div>

        <div className="shrink-0 border-t border-line-soft px-4 pb-4 pt-3">
          <div className="mb-3 flex items-center gap-2">
            <Button variant="default" size="icon" aria-label={playing ? "Pause" : "Play"} onClick={toggle}>
              {playing ? <Pause size={16} weight="fill" /> : <Play size={16} weight="fill" />}
            </Button>
            <Tip label="Previous frame (,)">
              <Button variant="ghost" size="icon-sm" onClick={() => seek(Math.max(0, time - 1 / fps))}>
                <CaretLeft size={14} />
              </Button>
            </Tip>
            <Tip label="Next frame (.)">
              <Button variant="ghost" size="icon-sm" onClick={() => seek(Math.min(duration, time + 1 / fps))}>
                <CaretRight size={14} />
              </Button>
            </Tip>
            <span className="ml-1 whitespace-nowrap text-base font-medium">{timecode(time, Math.round(fps))}</span>
            <span className="whitespace-nowrap text-sm text-faint">/ {timecode(duration, Math.round(fps))}</span>
            <div className="mx-2 h-5 w-px bg-line" />
            <div className="flex items-center rounded-ctl border border-line bg-well p-0.5">
              <Tip label={<span className="flex items-center gap-2">Start the selection here <Kbd keys="I" /></span>}>
                <button className="flex h-6 items-center gap-1.5 rounded-[4px] px-2 text-xs text-muted hover:bg-hover hover:text-fg" onClick={() => setInOut([Math.min(time, inOut[1] - 0.5), inOut[1]])}>
                  <ArrowLineLeft size={13} />
                  In
                </button>
              </Tip>
              <Tip label={<span className="flex items-center gap-2">End the selection here <Kbd keys="O" /></span>}>
                <button className="flex h-6 items-center gap-1.5 rounded-[4px] px-2 text-xs text-muted hover:bg-hover hover:text-fg" onClick={() => setInOut([inOut[0], Math.max(time, inOut[0] + 0.5)])}>
                  Out
                  <ArrowLineRight size={13} />
                </button>
              </Tip>
              <div className="mx-0.5 h-4 w-px bg-line" />
              <Tip label={<span className="flex items-center gap-2">Remove the selected part <Kbd keys="X" /></span>}>
                <button className="flex h-6 items-center gap-1.5 rounded-[4px] px-2 text-xs text-muted hover:bg-hover hover:text-fg" onClick={cutOut}>
                  <Scissors size={13} />
                  Cut
                </button>
              </Tip>
            </div>
            <span className="ml-1 whitespace-nowrap text-xs text-muted">
              {kept.toFixed(1)} s{cuts.length > 0 && <span className="text-faint">, {cuts.length === 1 ? "1 cut" : `${cuts.length} cuts`}</span>}
            </span>
            {(inOut[0] > 0.01 || inOut[1] < duration - 0.01 || cuts.length > 0) && (
              <button
                className="text-xs text-faint underline underline-offset-2 hover:text-fg"
                onClick={() => {
                  setInOut([0, duration]);
                  setCuts([]);
                }}
              >
                Reset
              </button>
            )}
            <div className="flex-1" />
            <Tip label="Loop selection">
              <Button variant="ghost" size="icon-sm" className={loop ? "text-fg" : ""} aria-pressed={loop} onClick={() => setLoop(!loop)}>
                <Repeat size={15} weight={loop ? "bold" : "regular"} />
              </Button>
            </Tip>
            <Button variant="ghost" size="icon-sm" aria-label={muted ? "Unmute" : "Mute"} onClick={() => setMuted(!muted)}>
              {muted || volume === 0 ? <SpeakerSlash size={15} /> : <SpeakerHigh size={15} />}
            </Button>
            <Slider className="w-24" min={0} max={1} step={0.01} value={[muted ? 0 : volume]} onValueChange={([v]) => (setVolume(v), setMuted(false))} />
            <Tip label={<span className="flex items-center gap-2">Shortcuts <Kbd keys="?" /></span>}>
              <Button variant="ghost" size="icon-sm" className="ml-1" onClick={() => useStore.setState({ shortcutsOpen: true })}>
                <Keyboard size={16} />
              </Button>
            </Tip>
          </div>
          <Timeline clip={clip} duration={duration} time={time} inOut={inOut} setInOut={setInOut} onSeek={seek} mix={mix} setMix={setMix} cuts={cuts} onRemoveCut={(i) => setCuts(cuts.filter((_, j) => j !== i))} />
        </div>
      </div>

      <aside className="flex w-[300px] shrink-0 flex-col border-l border-line-soft bg-canvas">
        <div className="flex items-baseline justify-between px-4 pb-1 pt-4">
          <h2 className="text-base font-semibold">Export</h2>
          <span className="text-xs text-muted">{kept.toFixed(1)} s</span>
        </div>
        <div className="min-h-0 flex-1">
          <ExportPanel
            clip={clip}
            draft={draft}
            setDraft={setDraft}
            length={kept}
            encode={encode}
            busy={lastJob?.status === "running" || lastJob?.status === "queued"}
            previewBusy={previewJob?.status === "running" || previewJob?.status === "queued"}
            onExport={() => submit(false)}
            onPreview={() => submit(true)}
          />
        </div>
        {lastJob && (lastJob.status === "running" || lastJob.status === "queued" || lastJob.status === "done") && (
          <div className="mx-4 mb-3 rounded-[8px] border border-line-soft bg-raised px-3 py-2.5">
            <div className="flex items-center justify-between text-xs">
              <span className="text-fg">{lastJob.status === "done" ? (lastJob.replace ? "Original replaced" : "Export saved") : "Exporting"}</span>
              {lastJob.status === "done" && !lastJob.replace ? (
                <span className="flex gap-3">
                  <button
                    className="text-muted underline underline-offset-2 hover:text-fg"
                    onClick={() => api.copyPath(lastJob.output).then(() => toast("success", "Copied. Paste it anywhere with Ctrl+V"))}
                  >
                    Copy
                  </button>
                  <button className="text-muted underline underline-offset-2 hover:text-fg" onClick={() => api.revealPath(lastJob.output)}>
                    Show in folder
                  </button>
                </span>
              ) : lastJob.status === "done" ? (
                <span className="text-muted">{lastJob.origSize > lastJob.size ? `${bytes(lastJob.origSize - lastJob.size)} smaller` : ""}</span>
              ) : (
                <span className="text-muted">{Math.round(lastJob.progress * 100)}%</span>
              )}
            </div>
            {lastJob.status !== "done" && (
              <div className="mt-2 h-[3px] overflow-hidden rounded-full bg-line">
                <div className="h-full bg-frost transition-[width]" style={{ width: `${lastJob.progress * 100}%` }} />
              </div>
            )}
          </div>
        )}
      </aside>

      <Dialog open={!!previewJob && previewJob.status === "done"} onOpenChange={(o) => !o && setPreviewId(null)} title="Motion blur preview" description="Three seconds from the middle of your selection." className="w-[760px]">
        {previewJob?.status === "done" && <video src={mediaUrl("job", previewJob.id)} autoPlay loop controls className={cn("mt-2 aspect-video w-full rounded-[6px] bg-black")} />}
      </Dialog>

      <Dialog open={showText} onOpenChange={setShowText} title="What was said" description="Searchable from the library." className="w-[520px]">
        <p className="mt-2 max-h-[50vh] overflow-y-auto whitespace-pre-wrap rounded-[6px] border border-line-soft bg-well p-3 text-sm leading-relaxed text-fg">{clip.transcript}</p>
      </Dialog>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete} title="Delete this clip?" description={`It goes to the recycle bin and frees ${bytes(clip.size)} once you empty it.`}>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setConfirmDelete(false)}>
            Keep it
          </Button>
          <Button
            variant="danger"
            onClick={async () => {
              await api.deleteClips([clip.id]);
              setConfirmDelete(false);
              go({ name: "library" });
            }}
          >
            Move to recycle bin
          </Button>
        </div>
      </Dialog>
    </div>
  );
}
