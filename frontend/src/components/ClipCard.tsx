import { ArrowsInSimple, CheckSquare, CloudArrowUp, LinkSimple, Copy, FilmSlate, FolderOpen, FolderSimple, Microphone, PencilSimple, Sparkle, Square, Star, Trash } from "@phosphor-icons/react";
import { useRef, useState, type MouseEvent } from "react";
import { CLIP_DRAG, folderMenu } from "@/features/library/Folders";
import fern from "@/assets/fern.jpg";
import live from "@/assets/live.jpg";
import paused from "@/assets/paused.jpg";
import setup from "@/assets/setup.jpg";
import waiting from "@/assets/waiting.jpg";

const mockThumbs = [live, setup, paused, fern, waiting];
import { aiReady, copyLink, nameWithAI, shrink, uploadClip } from "@/lib/actions";
import { api, type Clip } from "@/lib/api";
import { mockThumb } from "@/lib/mock";
import { bytes, clock, cn, relDate } from "@/lib/format";
import { mediaUrl, useStore } from "@/lib/store";
import { CtxMenu } from "./ui/menu";

export function Thumb({ clip, scrub = true, badge = true, className }: { clip: Clip; scrub?: boolean; badge?: boolean; className?: string }) {
  const [hoverX, setHoverX] = useState<number | null>(null);
  const ref = useRef<HTMLDivElement>(null);
  const mock = !useStore.getState().boot?.media.base;
  const thumb = clip.thumb ? mediaUrl("thumb", clip.id, clip.size) : "";
  const sprite = clip.sprite ? mediaUrl("sprite", clip.id, clip.size) : "";
  const frames = clip.spriteFrames || 10;
  const frame = hoverX === null ? -1 : Math.min(frames - 1, Math.floor(hoverX * frames));

  return (
    <div
      ref={ref}
      className={cn("relative aspect-video overflow-hidden rounded-[6px] bg-well", className)}
      onMouseMove={(e) => {
        if (!scrub || !sprite) return;
        const r = ref.current!.getBoundingClientRect();
        setHoverX(Math.max(0, Math.min(0.999, (e.clientX - r.left) / r.width)));
      }}
      onMouseLeave={() => setHoverX(null)}
    >
      {thumb ? (
        <img src={thumb} alt="" loading="lazy" draggable={false} className="absolute inset-0 h-full w-full object-cover" />
      ) : mock && mockThumb(clip) ? (
        <img src={mockThumb(clip)!} alt="" draggable={false} className="absolute inset-0 h-full w-full object-cover" />
      ) : mock ? (
        <div className="absolute inset-0" style={{ backgroundImage: `url(${mockThumbs[clip.id.charCodeAt(4) % mockThumbs.length]})`, backgroundSize: "220%", backgroundPosition: `${(clip.id.charCodeAt(4) * 37) % 100}% 50%` }} />
      ) : (
        <div className="absolute inset-0 grid place-items-center text-faint">
          <FilmSlate size={22} />
        </div>
      )}
      {frame >= 0 && sprite && (
        <div
          className="absolute inset-0"
          style={{ backgroundImage: `url(${sprite})`, backgroundSize: `${frames * 100}% 100%`, backgroundPosition: `${(frame / (frames - 1)) * 100}% 0` }}
        />
      )}
      {hoverX !== null && sprite && <div className="absolute bottom-0 left-0 h-[2px] bg-frost" style={{ width: `${hoverX * 100}%` }} />}
      {badge && <span className="absolute bottom-1.5 right-1.5 rounded-[4px] bg-black/70 px-1.5 py-px text-2xs font-medium text-white">{clock(clip.duration)}</span>}
    </div>
  );
}

export async function copyClip(id: string) {
  try {
    await api.copyClip(id);
    useStore.getState().toast("success", "Copied. Paste it anywhere with Ctrl+V");
  } catch (e) {
    useStore.getState().toast("error", String(e));
  }
}

// a plain element, the default ghost of a whole card covers the sidebar
function dragGhost(text: string) {
  const el = document.createElement("div");
  el.textContent = text;
  el.className = "fixed -top-20 left-0 max-w-[240px] truncate rounded-[6px] border border-line bg-raised px-2.5 py-1.5 text-sm text-fg";
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 0);
  return el;
}

export function ClipCard({ clip, selected, selecting, voiceHit, onToggleSelect, dragSet }: { clip: Clip; selected?: boolean; selecting?: boolean; voiceHit?: boolean; onToggleSelect?: (e?: MouseEvent) => void; dragSet?: () => string[] }) {
  const go = useStore((s) => s.go);
  const patch = useStore((s) => s.patchClip);
  const folders = useStore((s) => s.folders);
  const open = (e: MouseEvent) => (onToggleSelect && (selecting || e.ctrlKey || e.metaKey || e.shiftKey) ? onToggleSelect(e) : go({ name: "clip", id: clip.id }));
  const fav = async () => patch(await api.updateClip(clip.id, { favorite: !clip.favorite }));

  return (
    <CtxMenu
      items={[
        { label: "Open", icon: <PencilSimple size={15} />, onSelect: () => go({ name: "clip", id: clip.id }) },
        { label: clip.favorite ? "Remove favorite" : "Favorite", icon: <Star size={15} />, onSelect: fav },
        { label: "Move to folder", icon: <FolderSimple size={15} />, sub: folderMenu(folders, [clip.id], [clip.folder]) },
        { label: "Copy to clipboard", icon: <Copy size={15} />, onSelect: () => copyClip(clip.id) },
        { label: "Show in folder", icon: <FolderOpen size={15} />, onSelect: () => api.revealClip(clip.id) },
        { label: "Upload and copy link", icon: <CloudArrowUp size={15} />, onSelect: () => uploadClip(clip.id) },
        ...(clip.link ? [{ label: "Copy link", icon: <LinkSimple size={15} />, onSelect: () => copyLink(clip.link!) }] : []),
        ...(clip.deleteLink ? [{ label: "Copy delete link", icon: <LinkSimple size={15} />, onSelect: () => copyLink(clip.deleteLink!) }] : []),
        "sep",
        ...(aiReady() && !clip.export ? [{ label: "Name with AI", icon: <Sparkle size={15} />, onSelect: () => nameWithAI(clip.id) }] : []),
        { label: clip.shrunk ? "Already shrunk" : "Shrink file", icon: <ArrowsInSimple size={15} />, disabled: clip.shrunk, onSelect: () => shrink([clip.id]) },
        "sep",
        { label: "Move to recycle bin", icon: <Trash size={15} />, danger: true, onSelect: () => api.deleteClips([clip.id]) },
      ]}
    >
      <div
        className="group relative"
        draggable
        onDragStart={(e) => {
          const ids = selected && dragSet ? dragSet() : [clip.id];
          // drop whatever chromium attached, a Files entry makes wails block the drop
          e.dataTransfer.clearData();
          e.dataTransfer.setData(CLIP_DRAG, JSON.stringify(ids));
          e.dataTransfer.effectAllowed = "move";
          e.dataTransfer.setDragImage(dragGhost(ids.length === 1 ? clip.title : `${ids.length} clips`), 14, 14);
        }}
      >
        <button onClick={open} className={cn("block w-full rounded-[8px] text-left outline-offset-4", selected && "ring-2 ring-frost ring-offset-4 ring-offset-canvas")}>
          <Thumb clip={clip} />
          <div className="mt-2 flex items-start gap-2 px-0.5">
            <div className="min-w-0 flex-1">
              <p className="flex items-center gap-1.5 truncate text-sm font-medium text-fg">
                <span className="truncate">{clip.title}</span>
                {voiceHit && <Microphone size={12} className="shrink-0 text-muted" aria-label="Said in this clip" />}
              </p>
              <p className="mt-0.5 flex gap-3 text-xs text-muted">
                <span className="truncate">{clip.game || "Unknown"}</span>
                <span className="shrink-0">{relDate(clip.createdAt)}</span>
                <span className="ml-auto shrink-0 text-faint" title={clip.shrunk && clip.origSize ? `Was ${bytes(clip.origSize, 0)}` : undefined}>
                  {clip.shrunk && <ArrowsInSimple size={11} className="-mt-px mr-1 inline" />}
                  {bytes(clip.size, 0)}
                </span>
              </p>
            </div>
          </div>
        </button>
        <div className={cn("absolute left-2 top-2 flex gap-1 transition-opacity", selecting || selected ? "opacity-100" : "opacity-0 group-hover:opacity-100")}>
          {onToggleSelect && (
            <button aria-label={selected ? "Deselect" : "Select"} onClick={onToggleSelect} className="grid h-7 w-7 place-items-center rounded-[5px] bg-black/60 text-white hover:bg-black/80">
              {selected ? <CheckSquare size={16} weight="fill" /> : <Square size={16} />}
            </button>
          )}
        </div>
        <button
          aria-label={clip.favorite ? "Remove favorite" : "Favorite"}
          onClick={fav}
          className={cn("absolute right-2 top-2 grid h-7 w-7 place-items-center rounded-[5px] bg-black/60 text-white transition-opacity hover:bg-black/80", clip.favorite ? "opacity-100" : "opacity-0 group-hover:opacity-100")}
        >
          <Star size={15} weight={clip.favorite ? "fill" : "regular"} className={clip.favorite ? "text-warn" : ""} />
        </button>
      </div>
    </CtxMenu>
  );
}
