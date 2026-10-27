import { ArrowClockwise, Export, FilmStrip, FolderSimple, FolderSimplePlus, GearSix, House, PencilSimple, Plus, Star, Trash } from "@phosphor-icons/react";
import { CLIP_DRAG, dragIds, moveToFolder, openFolderDialog } from "@/features/library/Folders";
import { sections } from "@/features/settings/Settings";
import { useMemo, useState, type ButtonHTMLAttributes, type DragEvent, type ReactNode } from "react";
import { api, type Folder } from "@/lib/api";
import { bytes, cn } from "@/lib/format";
import { useStore, type LibraryKind } from "@/lib/store";
import { CtxMenu } from "./ui/menu";
import { Switch } from "./ui/switch";
import { Tip } from "./ui/tooltip";

// chromium hides drag data until drop, only the types are readable on hover
const draggingClips = (e: DragEvent) => e.dataTransfer.types.includes(CLIP_DRAG);

function DropRow({ children, onDropIds, className, ...rest }: { children: ReactNode; onDropIds: (ids: string[]) => void; className: string } & Omit<ButtonHTMLAttributes<HTMLButtonElement>, "className">) {
  const [over, setOver] = useState(false);
  return (
    <button
      {...rest}
      className={cn(className, over && "bg-hover text-fg ring-1 ring-inset ring-frost/50")}
      onDragOver={(e) => {
        if (!draggingClips(e)) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        setOver(false);
        const ids = dragIds(e);
        if (!ids?.length) return;
        e.preventDefault();
        onDropIds(ids);
      }}
    >
      {children}
    </button>
  );
}

const rowClass = (active: boolean) => cn("flex h-8 w-full items-center gap-2.5 rounded-[6px] px-2.5 text-sm transition-colors", active ? "bg-hover text-fg" : "text-muted hover:bg-hover/60 hover:text-fg");

function FolderRow({ folder, count, active, onOpen }: { folder: Folder; count: number; active: boolean; onOpen: () => void }) {
  return (
    <CtxMenu
      items={[
        { label: "Rename", icon: <PencilSimple size={15} />, onSelect: () => openFolderDialog({ mode: "rename", folder }) },
        { label: "Delete folder", icon: <Trash size={15} />, danger: true, onSelect: () => openFolderDialog({ mode: "delete", folder }) },
      ]}
    >
      <DropRow aria-current={active ? "page" : undefined} onClick={onOpen} onDropIds={(ids) => moveToFolder(ids, folder)} className={rowClass(active)}>
        <FolderSimple size={17} weight={active ? "fill" : "regular"} className="shrink-0" />
        <span className="flex-1 truncate text-left">{folder.name}</span>
        {count > 0 && <span className="text-xs text-faint">{count}</span>}
      </DropRow>
    </CtxMenu>
  );
}

function NavItem({ label, icon, active, count, onClick }: { label: string; icon: (active: boolean) => ReactNode; active: boolean; count?: number; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      aria-current={active ? "page" : undefined}
      className={cn("flex h-8 w-full items-center gap-2.5 rounded-[6px] px-2.5 text-sm transition-colors", active ? "bg-hover text-fg" : "text-muted hover:bg-hover/60 hover:text-fg")}
    >
      {icon(active)}
      <span className="flex-1 truncate text-left">{label}</span>
      {count !== undefined && count > 0 && <span className="text-xs text-faint">{count}</span>}
    </button>
  );
}

export function Sidebar() {
  const view = useStore((s) => s.view);
  const go = useStore((s) => s.go);
  const clips = useStore((s) => s.clips);
  const folders = useStore((s) => s.folders);
  const status = useStore((s) => s.status);
  const settingsActive = useStore((s) => s.settingsActive);
  const update = useStore((s) => s.update);
  const version = useStore((s) => s.boot?.version ?? "");
  const r = status?.recorder;
  const running = r?.state === "buffering" || r?.state === "starting";

  const counts = useMemo(() => {
    const games = new Map<string, number>();
    const inFolder = new Map<string, number>();
    let own = 0;
    let exports = 0;
    let favs = 0;
    for (const c of clips) {
      if (c.folder) inFolder.set(c.folder, (inFolder.get(c.folder) ?? 0) + 1);
      if (c.export) exports++;
      else {
        own++;
        const g = c.game || "Unknown";
        games.set(g, (games.get(g) ?? 0) + 1);
      }
      if (c.favorite) favs++;
    }
    return { own, exports, favs, inFolder, games: [...games.entries()].sort((a, b) => b[1] - a[1]).slice(0, 7) };
  }, [clips]);

  const lib = (kind: LibraryKind, game?: string) => go({ name: "library", kind, game });
  const isLib = (kind: LibraryKind, game?: string) => view.name === "library" && !view.folder && (view.kind ?? "clips") === kind && (view.game ?? "") === (game ?? "");
  const newFolder = (ids?: string[]) => openFolderDialog({ mode: "new", ids });
  const usage = status?.usage;
  const limit = usage?.budgetBytes || 0;
  const pct = usage ? Math.min(100, (usage.usedBytes / (limit || usage.usedBytes + usage.freeBytes || 1)) * 100) : 0;

  return (
    <nav className="flex w-[216px] shrink-0 flex-col border-r border-line-soft bg-canvas px-2.5 pb-3 pt-3">
      <div className="space-y-0.5">
        <NavItem label="Home" active={view.name === "home"} onClick={() => go({ name: "home" })} icon={(a) => <House size={17} weight={a ? "fill" : "regular"} />} />
        <NavItem label="Clips" count={counts.own} active={isLib("clips") || view.name === "clip"} onClick={() => lib("clips")} icon={(a) => <FilmStrip size={17} weight={a ? "fill" : "regular"} />} />
        <NavItem label="Exports" count={counts.exports} active={isLib("exports")} onClick={() => lib("exports")} icon={(a) => <Export size={17} weight={a ? "fill" : "regular"} />} />
        <NavItem label="Favorites" count={counts.favs} active={isLib("favorites")} onClick={() => lib("favorites")} icon={(a) => <Star size={17} weight={a ? "fill" : "regular"} />} />
      </div>

      {view.name === "settings" ? (
        <div className="mt-6 min-h-0 flex-1 overflow-y-auto">
          <p className="mb-1 px-2.5 text-xs text-faint">Settings</p>
          <div className="space-y-0.5">
            {sections.map(([id, label]) => (
              <button
                key={id}
                onClick={() => document.getElementById(`s-${id}`)?.scrollIntoView({ behavior: "smooth" })}
                className={cn(
                  "flex h-8 w-full items-center rounded-[6px] px-2.5 text-sm transition-colors",
                  settingsActive === id ? "bg-hover text-fg" : "text-muted hover:bg-hover/60 hover:text-fg",
                )}
              >
                {label}
              </button>
            ))}
          </div>
        </div>
      ) : (
        <div className="mt-6 min-h-0 flex-1 overflow-y-auto">
          <div className="mb-1 flex items-center justify-between pl-2.5 pr-1">
            <p className="text-xs text-faint">Folders</p>
            {folders.length > 0 && (
              <Tip label="New folder">
                <button aria-label="New folder" onClick={() => newFolder()} className="grid h-5 w-5 place-items-center rounded-[4px] text-faint hover:bg-hover hover:text-fg">
                  <Plus size={12} weight="bold" />
                </button>
              </Tip>
            )}
          </div>
          <div className="space-y-0.5">
            {folders.map((f) => (
              <FolderRow
                key={f.id}
                folder={f}
                count={counts.inFolder.get(f.id) ?? 0}
                active={view.name === "library" && view.folder === f.id}
                onOpen={() => go({ name: "library", kind: "all", folder: f.id })}
              />
            ))}
            {folders.length === 0 && (
              <DropRow onClick={() => newFolder()} onDropIds={(ids) => newFolder(ids)} className={rowClass(false)}>
                <FolderSimplePlus size={17} className="shrink-0" />
                <span className="flex-1 truncate text-left">New folder</span>
              </DropRow>
            )}
          </div>
          {counts.games.length > 0 && (
            <>
              <p className="mb-1 mt-5 px-2.5 text-xs text-faint">Games</p>
              <div className="space-y-0.5">
                {counts.games.map(([g, n]) => (
                  <button key={g} onClick={() => lib("clips", g)} className={rowClass(isLib("clips", g))}>
                    <span className="grid h-5 w-5 shrink-0 place-items-center rounded-[5px] bg-raised text-[10px] font-semibold text-fg ring-1 ring-line">{g.slice(0, 1).toUpperCase()}</span>
                    <span className="flex-1 truncate text-left">{g}</span>
                    <span className="text-xs text-faint">{n}</span>
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      <div className="mt-3 space-y-2">
        {usage && (
          <button onClick={() => go({ name: "settings", section: "storage" })} className="block w-full rounded-[8px] border border-line-soft px-3 py-2.5 text-left hover:border-line">
            <div className="flex items-baseline justify-between text-xs">
              <span className="text-fg">{bytes(usage.usedBytes)}</span>
              <span className="text-faint">{limit ? `of ${bytes(limit, 0)}` : `${bytes(usage.freeBytes, 0)} free`}</span>
            </div>
            <div className="mt-2 h-1 overflow-hidden rounded-full bg-line">
              <div className={cn("h-full", limit && pct > 90 ? "bg-warn" : "bg-frost")} style={{ width: `${Math.max(2, pct)}%` }} />
            </div>
          </button>
        )}
        <div className="flex items-center justify-between rounded-[8px] border border-line-soft px-3 py-2">
          <span className="flex items-center gap-2 text-sm">
            <span className={cn("h-[7px] w-[7px] rounded-full", running ? "live-dot bg-live" : "bg-faint")} />
            {running ? "Capturing" : r?.smartWaiting ? "Standing by" : "Paused"}
          </span>
          <Switch aria-label="Capturing" disabled={!status?.ffmpegReady} checked={running} onCheckedChange={(v) => (v ? api.startBuffer() : api.stopBuffer())} />
        </div>
        <NavItem label="Settings" active={view.name === "settings"} onClick={() => go({ name: "settings" })} icon={(a) => <GearSix size={17} weight={a ? "fill" : "regular"} />} />
        {update?.ready ? (
          <button
            onClick={() => api.restartToUpdate()}
            className="flex h-8 w-full items-center gap-2 rounded-[6px] bg-frost px-2.5 text-sm font-medium text-frost-ink hover:bg-frost-hi"
          >
            <ArrowClockwise size={15} weight="bold" />
            Restart to update
          </button>
        ) : (
          <p className="px-2.5 text-2xs text-faint">Snag {version}</p>
        )}
      </div>
    </nav>
  );
}
