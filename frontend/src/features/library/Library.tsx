import { ArrowsInSimple, CaretDown, CaretUp, FolderOpen, MagnifyingGlass, Stack, Trash, X } from "@phosphor-icons/react";
import { useMemo, useState } from "react";
import frost from "@/assets/frost.jpg";
import { ClipCard, Thumb } from "@/components/ClipCard";
import { VirtualGrid } from "@/components/VirtualGrid";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Kbd } from "@/components/ui/kbd";
import { Select } from "@/components/ui/select";
import { shrink } from "@/lib/actions";
import { api, type Clip } from "@/lib/api";
import { bytes, clock } from "@/lib/format";
import { useStore, type LibraryKind } from "@/lib/store";

type Sort = "new" | "old" | "big" | "long";

const titles: Record<LibraryKind, string> = { all: "Everything", clips: "Clips", exports: "Exports", favorites: "Favorites" };

export function Library({ kind = "clips", game }: { kind?: LibraryKind; game?: string }) {
  const clips = useStore((s) => s.clips);
  const go = useStore((s) => s.go);
  const hotkey = useStore((s) => s.settings?.hotkeys.saveClip ?? "F8");
  const [q, setQ] = useState("");
  const [sort, setSort] = useState<Sort>("new");
  const [sel, setSel] = useState<Set<string>>(new Set());
  const [confirm, setConfirm] = useState(false);
  const [merging, setMerging] = useState(false);

  const list = useMemo(() => {
    const needle = q.trim().toLowerCase();
    let out = clips.filter((c) => {
      if (kind === "clips" && c.export) return false;
      if (kind === "exports" && !c.export) return false;
      if (kind === "favorites" && !c.favorite) return false;
      if (game && (c.game || "Unknown") !== game) return false;
      if (needle && !`${c.title} ${c.game} ${(c.tags ?? []).join(" ")} ${c.transcript ?? ""}`.toLowerCase().includes(needle)) return false;
      return true;
    });
    out = [...out].sort((a, b) => {
      switch (sort) {
        case "old":
          return a.createdAt.localeCompare(b.createdAt);
        case "big":
          return b.size - a.size;
        case "long":
          return b.duration - a.duration;
        default:
          return b.createdAt.localeCompare(a.createdAt);
      }
    });
    return out;
  }, [clips, q, game, kind, sort]);

  const voiceHits = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return new Set<string>();
    return new Set(list.filter((c) => !`${c.title} ${c.game} ${(c.tags ?? []).join(" ")}`.toLowerCase().includes(needle)).map((c) => c.id));
  }, [list, q]);
  const shown = list.reduce((a, c) => a + c.size, 0);

  const toggle = (id: string) =>
    setSel((s) => {
      const n = new Set(s);
      n.has(id) ? n.delete(id) : n.add(id);
      return n;
    });
  const selectedBytes = clips.filter((c) => sel.has(c.id)).reduce((a, c) => a + c.size, 0);

  return (
    <div className="relative flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-3 border-b border-line-soft px-8 py-3">
        <h1 className="text-lg font-semibold">
          {game ? (
            <>
              <button className="text-muted hover:text-fg" onClick={() => go({ name: "library", kind })}>
                {titles[kind]}
              </button>
              <span className="mx-2 text-faint">/</span>
              {game}
            </>
          ) : (
            titles[kind]
          )}
        </h1>
        <span className="mr-2 text-sm text-faint">
          {list.length} {list.length === 1 ? "clip" : "clips"}, {bytes(shown)}
        </span>
        <div className="flex-1" />
        <div className="relative">
          <MagnifyingGlass size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-faint" />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={clips.some((c) => c.transcript) ? "Search titles, tags and voice" : "Search titles and tags"}
            className="h-8 w-[260px] rounded-ctl border border-line bg-well pl-8 pr-7 text-sm outline-none placeholder:text-faint hover:border-[#363a3f] focus:border-[#4a5260]"
          />
          {q && (
            <button aria-label="Clear search" onClick={() => setQ("")} className="absolute right-2 top-1/2 -translate-y-1/2 text-faint hover:text-fg">
              <X size={12} />
            </button>
          )}
        </div>
        <Select
          value={sort}
          onChange={(v) => setSort(v as Sort)}
          className="min-w-[140px]"
          options={[
            { value: "new", label: "Newest first" },
            { value: "old", label: "Oldest first" },
            { value: "big", label: "Largest first" },
            { value: "long", label: "Longest first" },
          ]}
        />
        <Button variant="ghost" size="icon" aria-label="Open clips folder" onClick={() => api.openClipsFolder()}>
          <FolderOpen size={17} />
        </Button>
      </div>

      <VirtualGrid
        items={list}
        minWidth={250}
        gapX={20}
        gapY={28}
        info={48}
        keyOf={(c) => c.id}
        className="min-h-0 flex-1 overflow-y-auto px-8 pb-10 pt-3"
        render={(c) => <ClipCard clip={c} voiceHit={voiceHits.has(c.id)} selected={sel.has(c.id)} selecting={sel.size > 0} onToggleSelect={() => toggle(c.id)} />}
      >
        {list.length === 0 ? (
          <div className="grid h-[60%] place-items-center text-center">
            {clips.length === 0 ? (
              <div className="relative w-full max-w-[640px] overflow-hidden rounded-card text-frost-ink">
                <img src={frost} alt="" className="absolute inset-0 h-full w-full object-cover" />
                <div className="relative px-8 py-12">
                  <p className="display text-2xl">Your library is empty</p>
                  <p className="mt-2 flex items-center justify-center gap-1.5 text-sm text-frost-sub">
                    Press <Kbd keys={hotkey} tone="frost" /> while capturing to save your first clip
                  </p>
                </div>
              </div>
            ) : (
              <div>
                <p className="text-base font-medium">Nothing matches</p>
                <button
                  className="mt-1.5 text-sm text-muted underline underline-offset-2 hover:text-fg"
                  onClick={() => {
                    setQ("");
                    go({ name: "library", kind: "all" });
                  }}
                >
                  Clear filters
                </button>
              </div>
            )}
          </div>
        ) : undefined}
      </VirtualGrid>

      {sel.size > 0 && (
        <div className="anim-toast absolute bottom-12 left-1/2 z-30 flex -translate-x-1/2 items-center gap-3 rounded-[10px] border border-line bg-[#1f2124] py-2 pl-4 pr-2 shadow-[0_16px_40px_rgba(0,0,0,0.5)]">
          <span className="text-sm">
            {sel.size} selected <span className="text-muted">{bytes(selectedBytes)}</span>
          </span>
          <Button size="sm" variant="ghost" onClick={() => setSel(new Set(list.map((c) => c.id)))}>
            Select all
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setSel(new Set())}>
            Cancel
          </Button>
          {sel.size > 1 && (
            <Button size="sm" variant="ghost" onClick={() => setMerging(true)}>
              <Stack size={14} />
              Merge
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            onClick={async () => {
              await shrink([...sel]);
              setSel(new Set());
            }}
          >
            <ArrowsInSimple size={14} />
            Shrink
          </Button>
          <Button size="sm" variant="danger" onClick={() => setConfirm(true)}>
            <Trash size={14} />
            Delete
          </Button>
        </div>
      )}

      <Dialog open={confirm} onOpenChange={setConfirm} title={`Delete ${sel.size} clip${sel.size === 1 ? "" : "s"}?`} description={`They go to the recycle bin and free ${bytes(selectedBytes)} once you empty it.`}>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setConfirm(false)}>
            Keep them
          </Button>
          <Button
            variant="danger"
            onClick={async () => {
              await api.deleteClips([...sel]);
              setSel(new Set());
              setConfirm(false);
            }}
          >
            Move to recycle bin
          </Button>
        </div>
      </Dialog>

      {merging && (
        <MergeDialog
          clips={clips.filter((c) => sel.has(c.id)).sort((a, b) => a.createdAt.localeCompare(b.createdAt))}
          onClose={(done) => {
            setMerging(false);
            if (done) setSel(new Set());
          }}
        />
      )}
    </div>
  );
}

function MergeDialog({ clips, onClose }: { clips: Clip[]; onClose: (done: boolean) => void }) {
  const [order, setOrder] = useState(clips);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const total = order.reduce((a, c) => a + c.duration, 0);
  const move = (i: number, d: number) =>
    setOrder((o) => {
      const n = [...o];
      [n[i], n[i + d]] = [n[i + d], n[i]];
      return n;
    });

  return (
    <Dialog open onOpenChange={(o) => !o && onClose(false)} title="Merge into one video" description={`${order.length} clips, ${clock(total)} total. Lands in Exports.`} className="w-[480px]">
      <div className="mt-2 max-h-[300px] space-y-1 overflow-y-auto">
        {order.map((c, i) => (
          <div key={c.id} className="flex items-center gap-3 rounded-[6px] border border-line-soft bg-well p-1.5 pr-2">
            <span className="w-4 text-center text-xs text-faint">{i + 1}</span>
            <Thumb clip={c} scrub={false} badge={false} className="w-[72px] shrink-0 rounded-[4px]" />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm">{c.title}</p>
              <p className="text-xs text-muted">{clock(c.duration)}</p>
            </div>
            <button aria-label="Move up" disabled={i === 0} onClick={() => move(i, -1)} className="rounded-[4px] p-1 text-muted hover:bg-hover hover:text-fg disabled:opacity-30">
              <CaretUp size={14} />
            </button>
            <button aria-label="Move down" disabled={i === order.length - 1} onClick={() => move(i, 1)} className="rounded-[4px] p-1 text-muted hover:bg-hover hover:text-fg disabled:opacity-30">
              <CaretDown size={14} />
            </button>
          </div>
        ))}
      </div>
      <label className="mt-4 block text-sm text-muted">
        Name
        <Input className="mt-1.5 w-full" value={name} placeholder="Montage" onChange={(e) => setName(e.target.value)} />
      </label>
      <div className="mt-5 flex justify-end gap-2">
        <Button variant="ghost" onClick={() => onClose(false)}>
          Cancel
        </Button>
        <Button
          variant="primary"
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            try {
              await api.mergeClips(
                order.map((c) => c.id),
                name.trim() || "Montage",
              );
              useStore.getState().toast("info", "Merging. It shows up in Exports when done");
              onClose(true);
            } catch (e) {
              useStore.getState().toast("error", String(e));
              setBusy(false);
            }
          }}
        >
          <Stack size={15} />
          Merge
        </Button>
      </div>
    </Dialog>
  );
}
