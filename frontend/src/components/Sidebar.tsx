import { ArrowClockwise, Export, FilmStrip, GearSix, House, Star } from "@phosphor-icons/react";
import { sections } from "@/features/settings/Settings";
import { useMemo, type ReactNode } from "react";
import { api } from "@/lib/api";
import { bytes, cn } from "@/lib/format";
import { useStore, type LibraryKind } from "@/lib/store";
import { Switch } from "./ui/switch";

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
  const status = useStore((s) => s.status);
  const settingsActive = useStore((s) => s.settingsActive);
  const update = useStore((s) => s.update);
  const version = useStore((s) => s.boot?.version ?? "");
  const r = status?.recorder;
  const running = r?.state === "buffering" || r?.state === "starting";

  const counts = useMemo(() => {
    const games = new Map<string, number>();
    let own = 0;
    let exports = 0;
    let favs = 0;
    for (const c of clips) {
      if (c.export) exports++;
      else {
        own++;
        const g = c.game || "Unknown";
        games.set(g, (games.get(g) ?? 0) + 1);
      }
      if (c.favorite) favs++;
    }
    return { own, exports, favs, games: [...games.entries()].sort((a, b) => b[1] - a[1]).slice(0, 7) };
  }, [clips]);

  const lib = (kind: LibraryKind, game?: string) => go({ name: "library", kind, game });
  const isLib = (kind: LibraryKind, game?: string) => view.name === "library" && (view.kind ?? "clips") === kind && (view.game ?? "") === (game ?? "");
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
      ) : counts.games.length > 0 ? (
        <div className="mt-6 min-h-0 flex-1 overflow-y-auto">
          <p className="mb-1 px-2.5 text-xs text-faint">Games</p>
          <div className="space-y-0.5">
            {counts.games.map(([g, n]) => (
              <button
                key={g}
                onClick={() => lib("clips", g)}
                className={cn(
                  "flex h-8 w-full items-center gap-2.5 rounded-[6px] px-2.5 text-sm transition-colors",
                  isLib("clips", g) ? "bg-hover text-fg" : "text-muted hover:bg-hover/60 hover:text-fg",
                )}
              >
                <span className="grid h-5 w-5 shrink-0 place-items-center rounded-[5px] bg-raised text-[10px] font-semibold text-fg ring-1 ring-line">{g.slice(0, 1).toUpperCase()}</span>
                <span className="flex-1 truncate text-left">{g}</span>
                <span className="text-xs text-faint">{n}</span>
              </button>
            ))}
          </div>
        </div>
      ) : (
        <div className="flex-1" />
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
