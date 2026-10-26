import { Check, FolderSimple, Minus, WarningCircle, X } from "@phosphor-icons/react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import live from "@/assets/live.jpg";
import { Mark } from "@/components/Logo";
import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { bytes, cn } from "@/lib/format";
import { setup, type Drive, type Info, type Paths, type Progress } from "./api";
import { Scene } from "./Scene";

type Step = "start" | "where" | "installing" | "done" | "failed" | "confirm" | "removing" | "removed";

function join(...p: string[]) {
  return p.map((s, i) => (i === 0 ? s.replace(/\\+$/, "") : s.replace(/^\\+|\\+$/g, ""))).join("\\");
}

function pathsFor(d: Drive, info: Info): Paths {
  if (d.system) {
    return { app: join(info.localAppData, "Programs", "Snag"), clips: join(info.home, "Videos", "Snag"), ffmpeg: join(info.localAppData, "Snag", "bin") };
  }
  const root = d.root.replace(/\\$/, "");
  return { app: join(root, "Snag", "App"), clips: join(root, "Snag", "Clips"), ffmpeg: join(root, "Snag", "FFmpeg") };
}

function Box({ label, hint, checked, onChange, danger }: { label: string; hint?: string; checked: boolean; onChange: (v: boolean) => void; danger?: boolean }) {
  return (
    <label className="flex cursor-pointer items-center gap-3 py-1.5 text-sm">
      <input type="checkbox" className="peer sr-only" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span
        className={cn(
          "grid h-4 w-4 shrink-0 place-items-center rounded-[4px] border transition-colors peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-frost",
          checked ? (danger ? "border-bad bg-bad text-frost-ink" : "border-frost bg-frost text-frost-ink") : "border-[#3a3e44] bg-well",
        )}
      >
        {checked && <Check size={11} weight="bold" />}
      </span>
      <span className={danger && checked ? "text-bad" : "text-fg"}>{label}</span>
      {hint && <span className="text-xs text-faint">{hint}</span>}
    </label>
  );
}

function Title({ children }: { children: ReactNode }) {
  return <h1 className="display text-[34px] leading-none tracking-[-0.015em]">{children}</h1>;
}

function Actions({ children }: { children: ReactNode }) {
  return <div className="flex items-center gap-2">{children}</div>;
}

export function Installer() {
  const [info, setInfo] = useState<Info | null>(null);
  const [step, setStep] = useState<Step>("start");
  const [drive, setDrive] = useState<Drive | null>(null);
  const [paths, setPaths] = useState<Paths>({ app: "", clips: "", ffmpeg: "" });
  const [opts, setOpts] = useState({ downloadFFmpeg: true, startWithWindows: true, desktopShortcut: false });
  const [removeClips, setRemoveClips] = useState(false);
  const [progress, setProgress] = useState<Progress>({ step: "", pct: 0, note: "", done: false, error: "" });

  useEffect(() => {
    setup.info().then((i) => {
      setInfo(i);
      if (i.uninstall) setStep("confirm");
      const most = [...i.drives].filter((d) => !d.removable).sort((a, b) => b.free - a.free)[0] ?? i.drives[0];
      if (!most) return;
      setDrive(most);
      const fresh = pathsFor(most, i);
      setPaths(i.update && i.existing.app ? { app: i.existing.app, clips: i.existing.clips || fresh.clips, ffmpeg: i.existing.ffmpeg || fresh.ffmpeg } : fresh);
    });
    return setup.onProgress((p) => {
      setProgress(p);
      if (p.error) setStep("failed");
      else if (p.done) setStep((s) => (s === "removing" ? "removed" : "done"));
    });
  }, []);

  const mostSpace = useMemo(() => (info ? [...info.drives].filter((d) => !d.removable).sort((a, b) => b.free - a.free)[0]?.root : ""), [info]);
  const clipsDrive = info?.drives.find((d) => paths.clips.toUpperCase().startsWith(d.root.toUpperCase()));
  const tight = !!clipsDrive && clipsDrive.free < 6 * 1024 ** 3;

  const depth = {
    start: 150,
    where: 175,
    installing: 175 + progress.pct * 140,
    done: 300,
    failed: 210,
    confirm: 190,
    removing: 190 - progress.pct * 120,
    removed: 70,
  }[step];

  const pick = async (key: keyof Paths) => {
    const p = await setup.pickFolder(paths[key]);
    if (p) setPaths({ ...paths, [key]: p });
  };

  const install = () => {
    if (!info) return;
    setProgress({ step: "", pct: 0, note: "", done: false, error: "" });
    setStep("installing");
    // updates keep the old choices
    const o = info.update ? { downloadFFmpeg: false, startWithWindows: info.autostart, desktopShortcut: false } : opts;
    setup.install({ paths, ...o, startMenu: true }).catch((e) => {
      setProgress((p) => ({ ...p, error: String(e) }));
      setStep("failed");
    });
  };

  let body: ReactNode = null;
  if (!info) body = null;
  else if (step === "start" && info.update)
    body = (
      <>
        <Title>Update Snag</Title>
        <p className="mt-4 text-sm text-muted">{info.installed && info.installed !== info.version ? `${info.installed} to ${info.version}` : info.version}</p>
        <div className="flex-1" />
        <Actions>
          <Button variant="primary" className="h-10 px-6 text-base" onClick={install}>
            Update
          </Button>
          <Button variant="ghost" className="h-10" onClick={() => setup.quit()}>
            Not now
          </Button>
        </Actions>
      </>
    );
  else if (step === "start")
    body = (
      <>
        <Title>Install Snag</Title>
        <button
          onClick={() => setStep("where")}
          className="group mt-7 flex items-center gap-3 rounded-[10px] border border-line bg-raised px-3.5 py-3 text-left transition-colors hover:border-[#3a3e44]"
        >
          <FolderSimple size={18} className="shrink-0 text-muted" />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm text-fg">{paths.clips}</span>
            <span className={cn("block text-xs", tight ? "text-warn" : "text-faint")}>{clipsDrive ? `${bytes(clipsDrive.free, 0)} free` : " "}</span>
          </span>
          <span className="text-xs text-muted group-hover:text-fg">Change</span>
        </button>
        <div className="mt-5">
          <Box label="Download FFmpeg" hint="100 MB" checked={opts.downloadFFmpeg} onChange={(v) => setOpts({ ...opts, downloadFFmpeg: v })} />
          <Box label="Start with Windows" checked={opts.startWithWindows} onChange={(v) => setOpts({ ...opts, startWithWindows: v })} />
          <Box label="Desktop shortcut" checked={opts.desktopShortcut} onChange={(v) => setOpts({ ...opts, desktopShortcut: v })} />
        </div>
        <div className="flex-1" />
        <div className="flex items-center justify-between">
          <Button variant="primary" className="h-10 px-6 text-base" onClick={install}>
            Install
          </Button>
          <span className="text-xs text-faint">{info.version}</span>
        </div>
      </>
    );
  else if (step === "where")
    body = (
      <>
        <Title>Where to</Title>
        <div className="mt-6 space-y-1.5">
          {info.drives.slice(0, 4).map((d) => {
            const low = d.free < 6 * 1024 ** 3;
            return (
              <button
                key={d.root}
                onClick={() => {
                  setDrive(d);
                  setPaths(pathsFor(d, info));
                }}
                className={cn(
                  "flex w-full items-center gap-3 rounded-[8px] border px-3 py-2 text-left transition-colors",
                  drive?.root === d.root ? "border-frost bg-white/[0.05]" : "border-line hover:border-[#3a3e44]",
                )}
              >
                <span className="display w-6 text-lg leading-none">{d.root.replace(/[:\\]/g, "")}</span>
                <span className="min-w-0 flex-1 truncate text-sm text-muted">{d.label || (d.removable ? "Removable" : "Local disk")}</span>
                {d.root === mostSpace && <span className="text-2xs text-faint">Most space</span>}
                <span className={cn("w-16 text-right text-sm", low ? "text-warn" : "text-fg")}>{bytes(d.free, 0)}</span>
              </button>
            );
          })}
        </div>
        <div className="mt-4 space-y-1 text-xs">
          {(["clips", "app", "ffmpeg"] as const).map((k) => (
            <div key={k} className="flex items-center gap-2">
              <span className="w-12 shrink-0 text-faint">{{ clips: "Clips", app: "App", ffmpeg: "FFmpeg" }[k]}</span>
              <span className="min-w-0 flex-1 truncate text-muted" title={paths[k]}>
                {paths[k]}
              </span>
              <button className="text-faint hover:text-fg" onClick={() => pick(k)}>
                Pick
              </button>
            </div>
          ))}
        </div>
        <div className="flex-1" />
        <Button variant="primary" className="h-10 self-start px-6 text-base" onClick={() => setStep("start")}>
          Done
        </Button>
      </>
    );
  else if (step === "installing" || step === "removing")
    body = (
      <>
        <Title>{step === "removing" ? "Removing" : info.update ? "Updating" : "Installing"}</Title>
        <div className="flex-1" />
        <p className="mb-2 h-5 truncate text-sm text-muted">{progress.note || "Starting"}</p>
        <div className="h-1 overflow-hidden rounded-full bg-line">
          <div className="h-full bg-frost transition-[width] duration-300" style={{ width: `${Math.max(2, progress.pct * 100)}%` }} />
        </div>
      </>
    );
  else if (step === "done")
    body = (
      <>
        <Title>{info.update ? "Updated" : "Installed"}</Title>
        {!info.update && (
          <p className="mt-4 flex items-center gap-1.5 text-sm text-muted">
            <Kbd keys="F8" /> in a game saves the last minute
          </p>
        )}
        <div className="flex-1" />
        <Actions>
          <Button variant="primary" className="h-10 px-6 text-base" onClick={() => setup.launch()}>
            Open Snag
          </Button>
          <Button variant="ghost" className="h-10" onClick={() => setup.quit()}>
            Close
          </Button>
        </Actions>
      </>
    );
  else if (step === "failed")
    body = (
      <>
        <Title>That didn't work</Title>
        <p className="mt-4 max-w-[380px] break-words text-sm text-muted">{progress.error}</p>
        <div className="flex-1" />
        <Actions>
          <Button variant="primary" className="h-10 px-6 text-base" onClick={install}>
            Try again
          </Button>
          <Button variant="ghost" className="h-10" onClick={() => setStep("start")}>
            Back
          </Button>
        </Actions>
      </>
    );
  else if (step === "confirm")
    body = (
      <>
        <Title>Remove Snag</Title>
        <div className="mt-6">
          <Box danger label="Also delete my clips" hint={info.clipCount ? `${info.clipCount} clips, ${bytes(info.clipBytes)}` : undefined} checked={removeClips} onChange={setRemoveClips} />
        </div>
        {removeClips && (
          <p className="mt-2 flex items-center gap-1.5 text-xs text-bad">
            <WarningCircle size={13} />
            Gone for good, not the recycle bin
          </p>
        )}
        <div className="flex-1" />
        <Actions>
          <Button
            variant="danger"
            className="h-10 px-6"
            onClick={() => {
              setStep("removing");
              setup.uninstall({ removeClips, removeData: true });
            }}
          >
            Remove
          </Button>
          <Button variant="ghost" className="h-10" onClick={() => setup.quit()}>
            Keep it
          </Button>
        </Actions>
      </>
    );
  else if (step === "removed")
    body = (
      <>
        <Title>Removed</Title>
        {!removeClips && info.clipCount > 0 && <p className="mt-4 text-sm text-muted">Your clips are still there</p>}
        <div className="flex-1" />
        <Button variant="primary" className="h-10 self-start px-6 text-base" onClick={() => setup.quit()}>
          Close
        </Button>
      </>
    );

  return (
    <div className="grid h-full grid-cols-[300px_1fr] bg-[#0e0f11]">
      <Scene images={[live]} active={0} depth={depth} draw={step === "start" || step === "confirm"} caught={step === "done"}>
        <div className="absolute left-5 top-4 text-white/90">
          <Mark size={16} />
        </div>
      </Scene>
      <div className="relative flex min-h-0 flex-col">
        <div className="drag flex h-10 shrink-0 justify-end">
          <button aria-label="Minimize" onClick={() => setup.minimise()} className="no-drag grid w-11 place-items-center text-muted hover:bg-hover hover:text-fg">
            <Minus size={14} />
          </button>
          <button
            aria-label="Close"
            disabled={step === "installing" || step === "removing"}
            onClick={() => setup.quit()}
            className="no-drag grid w-11 place-items-center text-muted hover:bg-[#c42b1c] hover:text-white disabled:opacity-30 disabled:hover:bg-transparent"
          >
            <X size={14} />
          </button>
        </div>
        <div key={step} className="rise flex min-h-0 flex-1 flex-col px-10 pb-9 pt-6">
          {body}
        </div>
      </div>
    </div>
  );
}
