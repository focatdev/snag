import { Segmented } from "@/components/ui/segmented";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import type { Encoder, Estimate, Settings as S } from "@/lib/api";
import { useStore } from "@/lib/store";
import { Row, Section, type SectionProps } from "../parts";

export function VideoSection({ s, set, est, encoders }: SectionProps & { est: Estimate | null; encoders: Encoder[] }) {
  const boot = useStore((st) => st.boot)!;
  return (
    <Section id="capture" title="Video">
      {est && (
        <div className="mb-2 mt-3 flex items-stretch overflow-hidden rounded-card border border-line-soft">
          {[
            ["Per minute", `${est.mbPerMin.toFixed(0)} MB`],
            ["Memory", `${est.bufferMB.toFixed(0)} MB`],
            ["Resolution", `${est.width}x${est.height}`],
            ["Bitrate", `${(est.kbps / 1000).toFixed(1)} Mbps`],
          ].map(([k, v]) => (
            <div key={k} className="flex-1 border-r border-line-soft px-4 py-3 last:border-r-0">
              <p className="text-2xs text-muted">{k}</p>
              <p className="mt-0.5 text-xl font-semibold">{v}</p>
            </div>
          ))}
        </div>
      )}
      <Row title="Encoder" desc="GPU options barely touch your CPU">
        <Select
          className="min-w-[200px]"
          value={s.encoder}
          onChange={(v) => set({ encoder: v })}
          options={[
            { value: "auto", label: "Auto" },
            ...encoders.filter((e) => e.codec !== "av1").map((e) => ({ value: e.name, label: e.label, disabled: !e.available, hint: e.available ? undefined : "Unavailable" })),
          ]}
        />
      </Row>
      <Row title="Format" desc="HEVC: about 30% smaller files">
        <Segmented
          value={s.codec}
          onChange={(v) => set({ codec: v })}
          options={[
            { value: "h264", label: "H.264" },
            { value: "hevc", label: "HEVC" },
          ]}
        />
      </Row>
      <Row title="Quality" desc="Higher looks better, takes more space">
        <Segmented
          value={s.quality}
          onChange={(v) => set({ quality: v })}
          options={[
            { value: "low", label: "Low" },
            { value: "medium", label: "Medium" },
            { value: "high", label: "High" },
            { value: "ultra", label: "Ultra" },
          ]}
        />
      </Row>
      <Row title="Resolution" desc="Lower saves space, costs a bit of CPU">
        <Select
          value={s.resolution}
          onChange={(v) => set({ resolution: v as S["resolution"] })}
          options={[
            { value: "native", label: "Match display" },
            { value: "1440", label: "1440p" },
            { value: "1080", label: "1080p" },
            { value: "720", label: "720p" },
          ]}
        />
      </Row>
      <Row title="Frame rate" desc="120+ for stronger motion blur">
        <Segmented
          value={s.fps}
          onChange={(v) => set({ fps: v })}
          options={[
            { value: 30, label: "30" },
            { value: 60, label: "60" },
            { value: 120, label: "120" },
            { value: 144, label: "144" },
          ]}
        />
      </Row>
      <Row title="Display">
        <Select
          value={String(s.monitor)}
          onChange={(v) => set({ monitor: Number(v) })}
          options={boot.monitors.map((m) => ({ value: String(m.index), label: `Display ${m.index + 1}`, hint: `${m.width}x${m.height}${m.primary ? ", main" : ""}` }))}
        />
      </Row>
      <Row title="Show mouse cursor">
        <Switch checked={s.drawCursor} onCheckedChange={(v) => set({ drawCursor: v })} />
      </Row>
    </Section>
  );
}
