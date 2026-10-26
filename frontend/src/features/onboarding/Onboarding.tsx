import { ArrowSquareOut, Check, CheckCircle, CircleNotch, DownloadSimple, FolderOpen, XCircle } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import fern from "@/assets/fern.jpg";
import setupImg from "@/assets/setup.jpg";
import waiting from "@/assets/waiting.jpg";
import { Mark } from "@/components/Logo";
import { Button } from "@/components/ui/button";
import { Row } from "@/components/ui/row";
import { Kbd } from "@/components/ui/kbd";
import { Switch } from "@/components/ui/switch";
import { api, openUrl, type Encoder } from "@/lib/api";
import { bytes, cn } from "@/lib/format";
import { hardwareHint } from "@/lib/gpu";
import { useStore } from "@/lib/store";

const steps = [
  { title: "Get FFmpeg", image: setupImg },
  { title: "Check your GPU", image: waiting },
  { title: "Save a clip", image: fern },
];

function Step({ n, active, done, label }: { n: number; active: boolean; done: boolean; label: string }) {
  return (
    <div className={cn("flex items-center gap-2.5 text-sm", active ? "text-fg" : done ? "text-muted" : "text-faint")}>
      <span
        className={cn(
          "grid h-6 w-6 place-items-center rounded-full border text-xs",
          active ? "border-frost bg-frost font-semibold text-frost-ink" : done ? "border-line bg-raised text-ok" : "border-line text-faint",
        )}
      >
        {done && !active ? <Check size={12} weight="bold" /> : n}
      </span>
      {label}
    </div>
  );
}

export function Onboarding() {
  const s = useStore((st) => st.settings)!;
  const save = useStore((st) => st.setSettings);
  const status = useStore((st) => st.status);
  const boot = useStore((st) => st.boot)!;
  const [step, setStep] = useState(0);
  const [encoders, setEncoders] = useState<Encoder[]>(boot.encoders ?? []);
  const [probing, setProbing] = useState(false);
  const ready = !!status?.ffmpegReady;
  const dl = status?.download;
  const set = (p: Partial<typeof s>) => save({ ...s, ...p });

  // backend probes after the download, just poll for it
  useEffect(() => {
    if (step !== 1 || !ready) return;
    let alive = true;
    const poll = () =>
      api
        .encoders()
        .then((e) => alive && setEncoders(e ?? []))
        .catch(() => {});
    poll();
    const t = window.setInterval(poll, 1500);
    return () => {
      alive = false;
      window.clearInterval(t);
    };
  }, [step, ready]);

  const finish = async (startNow: boolean) => {
    await save({ ...s, onboarded: true });
    if (startNow && ready) api.startBuffer().catch(() => {});
    useStore.getState().go({ name: "home" });
  };

  const gpus = boot.gpus ?? [];
  const hw = encoders.filter((e) => e.hardware && e.available && e.codec !== "av1");
  const hint = hardwareHint(encoders, gpus);
  const tested = encoders.length > 0;

  return (
    <div className="flex h-full">
      <div className="relative hidden w-[40%] max-w-[520px] overflow-hidden md:block">
        {steps.map((st, i) => (
          <img
            key={i}
            src={st.image}
            alt=""
            className={cn("absolute inset-0 h-full w-full object-cover transition-opacity duration-500", i === step ? "opacity-100" : "opacity-0")}
          />
        ))}
        <div className="absolute inset-0 bg-black/35" />
        <div className="absolute bottom-8 left-8 right-8 text-white">
          <Mark size={34} />
          <p className="display mt-4 text-3xl leading-[1.05]">Clips without the weight</p>
          <p className="mt-2 text-sm text-white/75">Three quick steps, then it stays out of your way.</p>
        </div>
      </div>

      <div className="flex min-w-0 flex-1 flex-col px-12 py-10">
        <div className="flex items-center gap-6">
          {steps.map((st, i) => (
            <Step key={i} n={i + 1} label={st.title} active={i === step} done={i < step} />
          ))}
          <div className="flex-1" />
          <button className="text-xs text-faint hover:text-fg" onClick={() => finish(false)}>
            Skip setup
          </button>
        </div>

        <div className="mt-12 max-w-[560px] flex-1">
          {step === 0 && (
            <>
              <h1 className="display text-3xl">Snag records with FFmpeg</h1>
              <p className="mt-2 text-sm text-muted">The free encoder most recording tools are built on. About 100 MB, and it can live off your system drive.</p>
              <div className="mt-8 rounded-card border border-line-soft bg-raised px-5">
                <Row
                  dense
                  title={ready ? `FFmpeg ${status?.ffmpeg.version ?? ""}` : "Not installed yet"}
                  desc={ready ? <span className="break-all">{status?.ffmpeg.ffmpeg}</span> : undefined}
                >
                  {ready ? (
                    <CheckCircle size={20} weight="fill" className="text-ok" />
                  ) : dl?.active ? (
                    <span className="text-xs text-muted">
                      {bytes(dl.done)} of {bytes(dl.total)}
                    </span>
                  ) : (
                    <div className="flex gap-2">
                      <Button
                        variant="ghost"
                        onClick={async () => {
                          const p = await api.pickFFmpeg();
                          if (p) set({ ffmpegPath: p });
                        }}
                      >
                        Use my own
                      </Button>
                      <Button variant="primary" onClick={() => api.downloadFFmpeg().catch((e) => useStore.getState().toast("error", String(e)))}>
                        <DownloadSimple size={15} />
                        Download
                      </Button>
                    </div>
                  )}
                </Row>
                {!ready && (
                  <Row dense title="Install folder" desc={<span className="break-all">{s.ffmpegDir || `${boot.dataDir}\\bin`}</span>}>
                    <Button
                      variant="ghost"
                      disabled={dl?.active}
                      onClick={async () => {
                        const p = await api.pickFolder(s.ffmpegDir || "");
                        if (p) set({ ffmpegDir: p });
                      }}
                    >
                      <FolderOpen size={15} />
                      Change
                    </Button>
                  </Row>
                )}
              </div>
              {dl?.active && (
                <div className="mt-4 h-1 overflow-hidden rounded-full bg-line">
                  <div className="h-full bg-frost transition-[width]" style={{ width: `${dl.total ? (dl.done / dl.total) * 100 : 0}%` }} />
                </div>
              )}
              {dl?.error && <p className="mt-3 text-sm text-bad">{dl.error}</p>}
            </>
          )}

          {step === 1 && (
            <>
              <h1 className="display text-3xl">{hw.length ? "Your GPU does the work" : tested ? "Your GPU didn't pass yet" : "Testing your GPU"}</h1>
              <p className="mt-2 text-sm text-muted">
                {hw.length ? "Capture barely touches your CPU, so games keep their frames." : "Snag can still record on the CPU, it just costs more while you play."}
              </p>
              <div className="mt-8 rounded-card border border-line-soft bg-raised px-5">
                {gpus.map((g) => (
                  <Row dense key={g.name} title={g.name} desc={g.driver ? `Driver ${g.driver}` : undefined}>
                    {!tested || probing ? (
                      <CircleNotch size={18} className="animate-spin text-muted" />
                    ) : encoders.some((e) => e.vendor === g.vendor && e.available && e.codec !== "av1") ? (
                      <CheckCircle size={20} weight="fill" className="text-ok" />
                    ) : (
                      <XCircle size={20} className="text-faint" />
                    )}
                  </Row>
                ))}
                <Row dense title="Encoder Snag will use" desc={hw.length ? hw.map((e) => e.label).join(", ") : tested ? "x264 on the CPU" : "Testing, takes up to a minute"}>
                  <Button
                    variant="ghost"
                    disabled={probing || !ready}
                    onClick={async () => {
                      setProbing(true);
                      setEncoders((await api.reprobe()) ?? []);
                      setProbing(false);
                    }}
                  >
                    Test again
                  </Button>
                </Row>
              </div>
              {hint && tested && !probing && (
                <div className="mt-4 rounded-[8px] border border-[#4a3d22] bg-[#221d13] px-3.5 py-3 text-xs text-warn">
                  <p>{hint.text}</p>
                  {hint.swap && (
                    <Button size="sm" variant="primary" className="mt-2" disabled={dl?.active} onClick={() => api.downloadFFmpeg().catch(() => {})}>
                      {dl?.active ? "Getting FFmpeg" : "Use an FFmpeg that fits my driver"}
                    </Button>
                  )}
                  {hint.url && (
                    <button className="mt-2 inline-flex items-center gap-1 text-fg underline underline-offset-2" onClick={() => openUrl(hint.url!)}>
                      {hint.link} <ArrowSquareOut size={11} />
                    </button>
                  )}
                </div>
              )}
            </>
          )}

          {step === 2 && (
            <>
              <h1 className="display text-3xl">Press a key, keep the moment</h1>
              <p className="mt-2 text-sm text-muted">
                Snag keeps the last {s.bufferSeconds === 60 ? "minute" : s.bufferSeconds % 60 === 0 ? `${s.bufferSeconds / 60} minutes` : `${s.bufferSeconds} seconds`} ready.
                Nothing becomes a file until you press the key.
              </p>
              <div className="mt-8 rounded-card border border-line-soft bg-raised px-5">
                <Row dense title="Save clip" desc="Works in fullscreen games">
                  <Kbd keys={s.hotkeys.saveClip} />
                </Row>
                <Row dense title={`Save the last ${s.shortSeconds} seconds`}>
                  <Kbd keys={s.hotkeys.saveShort} />
                </Row>
                <Row dense title="Screenshot">
                  <Kbd keys={s.hotkeys.screenshot} />
                </Row>
                <Row dense title="Start capturing when Snag opens">
                  <Switch checked={s.autoStartBuffer} onCheckedChange={(v) => set({ autoStartBuffer: v })} />
                </Row>
                <Row dense title="Start with Windows" desc="Opens quietly in the tray">
                  <Switch checked={s.launchOnStartup} onCheckedChange={(v) => set({ launchOnStartup: v })} />
                </Row>
              </div>
              <p className="mt-3 text-xs text-faint">Change any of these in Settings later.</p>
            </>
          )}
        </div>

        <div className="flex items-center justify-between">
          {step > 0 ? (
            <Button variant="ghost" onClick={() => setStep(step - 1)}>
              Back
            </Button>
          ) : (
            <span />
          )}
          {step < 2 ? (
            <Button variant="primary" disabled={step === 0 && !ready} onClick={() => setStep(step + 1)}>
              Next
            </Button>
          ) : (
            <Button variant="primary" onClick={() => finish(true)}>
              {ready ? "Start capturing" : "Done"}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
