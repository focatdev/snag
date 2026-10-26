import { CircleNotch, Export, Play, Scissors } from "@phosphor-icons/react";
import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/segmented";
import { Select } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import type { Clip, ExportOptions } from "@/lib/api";
import { bytes, cn } from "@/lib/format";

export type Draft = Omit<ExportOptions, "clipId" | "start" | "end" | "tracks" | "preview" | "name" | "cuts">;

export const defaultDraft: Draft = {
  fast: true,
  format: "mp4",
  targetMB: 0,
  height: 0,
  fps: 0,
  speed: 1,
  motionBlur: "off",
  vertical: false,
  cropX: 0.5,
  mixAudio: true,
  replaceOriginal: false,
};

function Field({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="py-2.5">
      <div className="mb-1.5 flex items-baseline justify-between">
        <span className="text-xs text-muted">{label}</span>
        {hint && <span className="text-2xs text-faint">{hint}</span>}
      </div>
      {children}
    </div>
  );
}

// keep in sync with canCopy in jobs.go
export function needsEncode(d: Draft, mixed: boolean, volumesChanged: boolean, cuts: boolean) {
  return cuts || d.format !== "mp4" || d.targetMB > 0 || d.height > 0 || d.fps > 0 || d.speed !== 1 || d.vertical || d.motionBlur !== "off" || mixed || volumesChanged;
}

export function ExportPanel({
  clip,
  draft,
  setDraft,
  length,
  encode,
  busy,
  previewBusy,
  onExport,
  onPreview,
}: {
  clip: Clip;
  draft: Draft;
  setDraft: (d: Draft) => void;
  length: number;
  encode: boolean;
  busy: boolean;
  previewBusy: boolean;
  onExport: () => void;
  onPreview: () => void;
}) {
  const set = (p: Partial<Draft>) => setDraft({ ...draft, ...p });
  const tracks = clip.tracks?.length ?? 0;
  const outLen = length / draft.speed;

  let estimate = "";
  if (draft.targetMB > 0) estimate = `under ${draft.targetMB} MB`;
  else if (!encode) estimate = bytes((clip.size * length) / Math.max(clip.duration, 0.01));
  else if (draft.format === "gif") estimate = bytes(outLen * 0.6 * 1024 * 1024);
  else {
    const h = draft.height || clip.height || 1080;
    const mbps = 14 * Math.pow((h * h * 16) / 9 / 2073600, 0.75);
    estimate = `about ${bytes(((mbps * 1000 + 128) * outLen * 1000) / 8)}`;
  }

  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
        <Field label="Format">
          <Segmented
            className="w-full"
            value={draft.format}
            onChange={(v) => set({ format: v })}
            options={[
              { value: "mp4", label: "MP4 video" },
              { value: "gif", label: "GIF" },
            ]}
          />
        </Field>

        {draft.format === "mp4" && (
          <Field label="Size limit" hint={draft.targetMB === 10 ? "Discord free upload limit" : draft.targetMB === 50 ? "Discord Nitro Basic" : draft.targetMB === 500 ? "Discord Nitro" : undefined}>
            <Select
              className="w-full"
              value={String(draft.targetMB)}
              onChange={(v) => set({ targetMB: Number(v) })}
              options={[
                { value: "0", label: "No limit" },
                { value: "10", label: "10 MB" },
                { value: "25", label: "25 MB" },
                { value: "50", label: "50 MB" },
                { value: "100", label: "100 MB" },
                { value: "500", label: "500 MB" },
              ]}
            />
          </Field>
        )}

        <div className="grid grid-cols-2 gap-3">
          <Field label="Resolution">
            <Select
              className="w-full min-w-0"
              value={String(draft.height)}
              onChange={(v) => set({ height: Number(v) })}
              options={[
                { value: "0", label: draft.targetMB > 0 ? "Automatic" : "Original" },
                ...[1440, 1080, 720, 480].filter((h) => h < (clip.height || 9999)).map((h) => ({ value: String(h), label: `${h}p` })),
              ]}
            />
          </Field>
          <Field label="Frame rate">
            <Select
              className="w-full min-w-0"
              value={String(draft.fps)}
              onChange={(v) => set({ fps: Number(v) })}
              options={[{ value: "0", label: "Original" }, ...[60, 30].filter((f) => f < (clip.fps || 999) - 1).map((f) => ({ value: String(f), label: `${f} fps` }))]}
            />
          </Field>
        </div>

        <Field label="Speed">
          <Segmented
            className="w-full"
            size="sm"
            value={draft.speed}
            onChange={(v) => set({ speed: v })}
            options={[
              { value: 0.5, label: "0.5x" },
              { value: 1, label: "1x" },
              { value: 1.5, label: "1.5x" },
              { value: 2, label: "2x" },
            ]}
          />
        </Field>

        <Field label="Motion blur" hint={clip.fps < 100 && draft.motionBlur !== "off" ? "Stronger on 120 fps recordings" : undefined}>
          <Segmented
            className="w-full"
            size="sm"
            value={draft.motionBlur}
            onChange={(v) => set({ motionBlur: v })}
            options={[
              { value: "off", label: "Off" },
              { value: "subtle", label: "Subtle" },
              { value: "medium", label: "Medium" },
              { value: "heavy", label: "Heavy" },
            ]}
          />
          {draft.motionBlur !== "off" && (
            <Button size="sm" variant="default" className="mt-2 w-full justify-center" disabled={previewBusy} onClick={onPreview}>
              {previewBusy ? <CircleNotch size={13} className="animate-spin" /> : <Play size={13} weight="fill" />}
              {previewBusy ? "Rendering preview" : "Preview 3 seconds"}
            </Button>
          )}
        </Field>

        <div className="flex items-center justify-between py-2.5">
          <div>
            <p className="text-sm text-fg">Vertical 9:16</p>
            <p className="text-2xs text-faint">For Shorts, TikTok and Reels</p>
          </div>
          <Switch checked={draft.vertical} onCheckedChange={(v) => set({ vertical: v })} />
        </div>
        {draft.vertical && (
          <Field label="Crop position" hint="Or drag the frame on the video">
            <Slider min={0} max={1} step={0.01} value={[draft.cropX]} onValueChange={([v]) => set({ cropX: v })} />
          </Field>
        )}

        {draft.format === "mp4" && !clip.export && (
          <div className="flex items-center justify-between py-2.5">
            <div>
              <p className="text-sm text-fg">Replace original</p>
              <p className="text-2xs text-faint">{draft.replaceOriginal ? "Original goes to the recycle bin" : "Keeps both files"}</p>
            </div>
            <Switch checked={draft.replaceOriginal} onCheckedChange={(v) => set({ replaceOriginal: v })} />
          </div>
        )}

        {tracks > 1 && draft.format === "mp4" && (
          <div className="flex items-center justify-between py-2.5">
            <div>
              <p className="text-sm text-fg">Mix audio tracks</p>
              <p className="text-2xs text-faint">Most sites only play one track</p>
            </div>
            <Switch checked={draft.mixAudio} onCheckedChange={(v) => set({ mixAudio: v })} />
          </div>
        )}
      </div>

      <div className="border-t border-line-soft p-4">
        <div className="mb-3 flex justify-between text-xs">
          <span className="text-muted">{encode ? "Takes a moment to process" : "Instant, original quality"}</span>
          <span className="text-fg">{estimate}</span>
        </div>
        <Button variant="primary" className={cn("w-full justify-center")} disabled={busy} onClick={onExport}>
          {encode ? <Export size={16} /> : <Scissors size={16} />}
          {draft.replaceOriginal && draft.format === "mp4" ? (encode ? "Export and replace" : "Trim original") : encode ? "Export clip" : "Save trimmed copy"}
        </Button>
        {!encode && <p className="mt-2 text-center text-2xs text-faint">Cuts land within about a second</p>}
      </div>
    </div>
  );
}
