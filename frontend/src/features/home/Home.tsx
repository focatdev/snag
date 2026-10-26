import { Warning } from "@phosphor-icons/react";
import fern from "@/assets/fern.jpg";
import { ClipCard } from "@/components/ClipCard";
import { Kbd } from "@/components/ui/kbd";
import { bytes, resLabel } from "@/lib/format";
import { useStore } from "@/lib/store";
import { BufferCard, SetupCard } from "./BufferCard";

export function Home() {
  const st = useStore((s) => s.status);
  const settings = useStore((s) => s.settings);
  const clips = useStore((s) => s.clips);
  const go = useStore((s) => s.go);
  if (!st || !settings) return null;
  const r = st.recorder;
  const live = r.state === "buffering";
  const recent = clips.filter((c) => !c.export).slice(0, 4);
  const weekAgo = Date.now() - 7 * 86400_000;
  const thisWeek = clips.filter((c) => !c.export && new Date(c.createdAt).getTime() > weekAgo);

  const stats: [string, string, string?][] = [
    ["Recording", live ? `${resLabel(r.height)} at ${r.fps} fps` : "Off", live ? r.encoderLabel : undefined],
    ["CPU", `${r.cpu.toFixed(1)}%`, "Snag and FFmpeg"],
    ["Memory", bytes(r.memBytes, 0)],
    ["This week", `${thisWeek.length} clip${thisWeek.length === 1 ? "" : "s"}`, bytes(thisWeek.reduce((a, c) => a + c.size, 0))],
    ["Free space", bytes(st.usage.freeBytes, 0), st.usage.budgetBytes > 0 ? `limit ${bytes(st.usage.budgetBytes, 0)}` : undefined],
  ];

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-[1180px] px-8 pb-10 pt-7">
        {st.ffmpegReady ? <BufferCard /> : <SetupCard />}

        <dl className="mt-4 grid grid-cols-5 divide-x divide-line-soft rounded-card border border-line-soft">
          {stats.map(([k, v, sub]) => (
            <div key={k} className="px-4 py-3">
              <dt className="text-xs text-muted">{k}</dt>
              <dd className="mt-0.5 truncate text-sm text-fg">{v}</dd>
              <dd className="truncate text-2xs text-faint">{sub ?? " "}</dd>
            </div>
          ))}
        </dl>

        {st.hotkeyErrors.length > 0 && (
          <div className="mt-4 flex items-start gap-2.5 rounded-[8px] border border-[#4a3d22] bg-[#221d13] px-3.5 py-2.5 text-sm text-warn">
            <Warning size={16} className="mt-0.5 shrink-0" />
            <div>
              <p>Some hotkeys are taken by another app</p>
              <p className="mt-0.5 text-xs text-warn/70">{st.hotkeyErrors.join(", ")}</p>
            </div>
            <button className="ml-auto shrink-0 text-xs underline underline-offset-2" onClick={() => go({ name: "settings" })}>
              Change hotkeys
            </button>
          </div>
        )}

        <div className="mb-3 mt-9 flex items-baseline justify-between">
          <h2 className="text-base font-semibold">Recent clips</h2>
          {clips.length > 0 && (
            <button className="text-sm text-muted hover:text-fg" onClick={() => go({ name: "library" })}>
              Open library
            </button>
          )}
        </div>
        {recent.length === 0 ? (
          <div className="relative overflow-hidden rounded-card bg-black">
            <img src={fern} alt="" className="absolute inset-0 h-full w-full object-cover opacity-50" />
            <div className="relative flex items-center gap-2 px-6 py-9 text-sm text-white/85">
              <span className="text-base font-medium text-white">No clips yet.</span>
              Press <Kbd keys={settings.hotkeys.saveClip} /> in a game and it lands here.
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-4 gap-x-4 gap-y-6">
            {recent.map((c) => (
              <ClipCard key={c.id} clip={c} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
