import { useEffect, useState, type ReactNode } from "react";
import { useStore } from "@/lib/store";
import { Button } from "./ui/button";
import { Dialog } from "./ui/dialog";
import { Kbd } from "./ui/kbd";

function Line({ label, keys }: { label: string; keys: ReactNode }) {
  return (
    <div className="flex items-center justify-between py-1.5 text-sm">
      <span className="text-muted">{label}</span>
      <span className="flex items-center gap-1.5">{keys}</span>
    </div>
  );
}

export function Shortcuts() {
  const open = useStore((s) => s.shortcutsOpen);
  const hk = useStore((s) => s.settings?.hotkeys);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest("input, textarea")) return;
      if (e.key === "?") useStore.setState({ shortcutsOpen: !useStore.getState().shortcutsOpen });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <Dialog open={open} onOpenChange={(o) => useStore.setState({ shortcutsOpen: o })} title="Shortcuts" className="w-[560px]">
      <div className="mt-2 grid grid-cols-2 gap-x-8">
        <div>
          <p className="mb-1 text-xs text-faint">Anywhere, even in games</p>
          <Line label="Save clip" keys={<Kbd keys={hk?.saveClip ?? ""} />} />
          <Line label="Save short clip" keys={<Kbd keys={hk?.saveShort ?? ""} />} />
          <Line label="Screenshot" keys={<Kbd keys={hk?.screenshot ?? ""} />} />
          <Line label="Bookmark" keys={<Kbd keys={hk?.bookmark ?? ""} />} />
          <Line label="Start or stop" keys={<Kbd keys={hk?.toggleBuffer ?? ""} />} />
          <p className="mb-1 mt-4 text-xs text-faint">In Snag</p>
          <Line label="This sheet" keys={<Kbd keys="?" />} />
        </div>
        <div>
          <p className="mb-1 text-xs text-faint">Clip editor</p>
          <Line label="Play or pause" keys={<><Kbd keys="Space" /><Kbd keys="K" /></>} />
          <Line label="Back or ahead 1 s" keys={<><Kbd keys="J" /><Kbd keys="L" /></>} />
          <Line label="Back or ahead 5 s" keys={<Kbd keys="Shift+J" />} />
          <Line label="One frame" keys={<><Kbd keys="," /><Kbd keys="." /></>} />
          <Line label="Selection in and out" keys={<><Kbd keys="I" /><Kbd keys="O" /></>} />
          <Line label="Cut the selection" keys={<Kbd keys="X" />} />
          <Line label="Mute" keys={<Kbd keys="M" />} />
          <Line label="Back to library" keys={<Kbd keys="Esc" />} />
        </div>
      </div>
    </Dialog>
  );
}

// just enough markdown for github release notes
function Notes({ text }: { text: string }) {
  const lines = text.replace(/\r/g, "").split("\n");
  return (
    <div className="max-h-[50vh] space-y-1.5 overflow-y-auto text-sm leading-relaxed text-fg">
      {lines.map((l, i) => {
        const t = l.trim();
        if (!t) return <div key={i} className="h-1" />;
        const clean = t.replace(/\*\*(.+?)\*\*/g, "$1").replace(/`(.+?)`/g, "$1").replace(/\[(.+?)\]\(.+?\)/g, "$1");
        if (/^#{1,6}\s/.test(t)) return <p key={i} className="pt-2 font-semibold">{clean.replace(/^#+\s*/, "")}</p>;
        if (/^[-*]\s/.test(t))
          return (
            <p key={i} className="flex gap-2 text-muted">
              <span className="text-faint">-</span>
              <span>{clean.replace(/^[-*]\s*/, "")}</span>
            </p>
          );
        return (
          <p key={i} className="text-muted">
            {clean}
          </p>
        );
      })}
    </div>
  );
}

export function WhatsNew() {
  const w = useStore((s) => s.boot?.whatsNew);
  // mounts before bootstrap arrives, so open can't be seeded from w
  const [closed, setClosed] = useState(false);
  if (!w) return null;
  const setOpen = (o: boolean) => setClosed(!o);
  return (
    <Dialog open={!closed} onOpenChange={setOpen} title={`What's new in ${w.version}`} className="w-[520px]">
      <div className="mt-2">{w.notes.trim() ? <Notes text={w.notes} /> : <p className="text-sm text-muted">Fixes and small improvements.</p>}</div>
      <div className="mt-5 flex justify-end">
        <Button variant="primary" onClick={() => setOpen(false)}>
          Nice
        </Button>
      </div>
    </Dialog>
  );
}
