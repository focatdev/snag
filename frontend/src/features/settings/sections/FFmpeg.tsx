import { ArrowClockwise, ArrowSquareOut, CheckCircle, XCircle } from "@phosphor-icons/react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { api, openUrl, type Encoder } from "@/lib/api";
import { hardwareHint } from "@/lib/gpu";
import { useStore } from "@/lib/store";
import { Row, Section, type SectionProps } from "../parts";

export function FFmpegSection({ s, set, encoders, setEncoders }: SectionProps & { encoders: Encoder[]; setEncoders: (e: Encoder[]) => void }) {
  const boot = useStore((st) => st.boot)!;
  const status = useStore((st) => st.status);
  const [probing, setProbing] = useState(false);
  const gpuVendors = new Set((boot.gpus ?? []).map((g) => g.vendor));
  const gpuHint = hardwareHint(encoders, boot.gpus ?? []);
  return (
    <Section id="ffmpeg" title="FFmpeg">
      <Row
        title={status?.ffmpegReady ? `FFmpeg ${status.ffmpeg.version}` : "FFmpeg not found"}
        desc={<span className="break-all">{status?.ffmpeg.ffmpeg || "Not installed yet"}</span>}
      >
        <div className="flex gap-2">
          <Button
            onClick={async () => {
              const p = await api.pickFFmpeg();
              if (p) set({ ffmpegPath: p });
            }}
          >
            Use existing
          </Button>
          <Button onClick={() => api.downloadFFmpeg().catch((e) => useStore.getState().toast("error", String(e)))} disabled={status?.download.active}>
            {status?.download.active
              ? `Downloading ${status.download.total ? Math.round((status.download.done / status.download.total) * 100) : 0}%`
              : status?.ffmpegReady
                ? "Update"
                : "Download"}
          </Button>
        </div>
      </Row>
      <Row title="Install folder" desc={<span className="break-all">{s.ffmpegDir || `${boot.dataDir}\\bin`}</span>}>
        <div className="flex gap-2">
          {s.ffmpegDir && (
            <Button variant="ghost" onClick={() => set({ ffmpegDir: "" })}>
              Reset
            </Button>
          )}
          <Button
            onClick={async () => {
              const p = await api.pickFolder(s.ffmpegDir || "");
              if (p) set({ ffmpegDir: p });
            }}
          >
            Change
          </Button>
        </div>
      </Row>
      <Row
        title="Encoders"
        desc={(boot.gpus ?? []).length ? (boot.gpus ?? []).map((g) => `${g.name}${g.driver ? `, driver ${g.driver}` : ""}`).join(" and ") : "Tested on this PC"}
        wide
      >
        <div className="grid grid-cols-2 gap-x-6 gap-y-1.5">
          {encoders
            .filter((e) => !e.hardware || e.available || gpuVendors.has(e.vendor) || gpuVendors.size === 0)
            .map((e) => (
              <div key={e.name} className="min-w-0 text-sm" title={e.error}>
                <div className="flex items-center gap-2">
                  {e.available ? <CheckCircle size={14} weight="fill" className="shrink-0 text-ok" /> : <XCircle size={14} className="shrink-0 text-faint" />}
                  <span className={e.available ? "text-fg" : "text-faint"}>{e.label}</span>
                </div>
                {!e.available && e.error && e.hardware && <p className="ml-[22px] truncate text-2xs text-muted">{e.error}</p>}
              </div>
            ))}
          {encoders.length === 0 && <p className="text-sm text-muted">Not tested yet.</p>}
        </div>
        <Button
          size="sm"
          className="mt-3 w-fit"
          disabled={probing || !status?.ffmpegReady}
          onClick={async () => {
            setProbing(true);
            setEncoders((await api.reprobe()) ?? []);
            setProbing(false);
          }}
        >
          <ArrowClockwise size={13} className={probing ? "animate-spin" : ""} />
          {probing ? "Testing, takes up to a minute" : "Test again"}
        </Button>
        {gpuHint && (
          <div className="mt-3 rounded-[8px] border border-[#4a3d22] bg-[#221d13] px-3 py-2.5 text-xs text-warn">
            <p>{gpuHint.text}</p>
            <div className="mt-1.5 flex items-center gap-4">
              {gpuHint.swap && (
                <Button
                  size="sm"
                  variant="primary"
                  disabled={status?.download.active}
                  onClick={() => api.downloadFFmpeg().catch((e) => useStore.getState().toast("error", String(e)))}
                >
                  {status?.download.active
                    ? `Getting FFmpeg ${status.download.total ? Math.round((status.download.done / status.download.total) * 100) : 0}%`
                    : "Use an FFmpeg that fits my driver"}
                </Button>
              )}
              {gpuHint.url && (
                <button className="inline-flex items-center gap-1 text-fg underline underline-offset-2" onClick={() => openUrl(gpuHint.url!)}>
                  {gpuHint.link} <ArrowSquareOut size={11} />
                </button>
              )}
              {gpuHint.detail && (
                <button
                  className="text-fg underline underline-offset-2"
                  onClick={() =>
                    navigator.clipboard.writeText(gpuHint.detail!).then(
                      () => useStore.getState().toast("success", "Details copied"),
                      () => useStore.getState().toast("error", "Couldn't reach the clipboard"),
                    )
                  }
                >
                  Copy details
                </button>
              )}
            </div>
          </div>
        )}
      </Row>
    </Section>
  );
}
