import { Play } from "@phosphor-icons/react";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/segmented";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { api, type Estimate } from "@/lib/api";
import { clock } from "@/lib/format";
import { useStore } from "@/lib/store";
import { Row, Section, type SectionProps } from "../parts";

export function ClipsSection({ s, set, est }: SectionProps & { est: Estimate | null }) {
  return (
    <Section id="buffer" title="Clips">
      <Row title="Clip length" desc={s.bufferOnDisk ? "How far back a save reaches" : "How far back a save reaches, kept in memory"}>
        <div className="flex w-[260px] items-center gap-3">
          <Slider
            min={15}
            max={300}
            step={15}
            value={[s.bufferSeconds]}
            onValueChange={([v]) => useStore.setState({ settings: { ...s, bufferSeconds: v } })}
            onValueCommit={([v]) => set({ bufferSeconds: v })}
          />
          <span className="w-12 text-right text-base font-medium">{clock(s.bufferSeconds)}</span>
        </div>
      </Row>
      <Row title="Short clip length" desc={`What ${s.hotkeys.saveShort || "the short clip hotkey"} saves`}>
        <div className="flex w-[260px] items-center gap-3">
          <Slider
            min={5}
            max={Math.min(120, s.bufferSeconds)}
            step={5}
            value={[Math.min(s.shortSeconds, s.bufferSeconds)]}
            onValueChange={([v]) => useStore.setState({ settings: { ...s, shortSeconds: v } })}
            onValueCommit={([v]) => set({ shortSeconds: v })}
          />
          <span className="w-12 text-right text-base font-medium">{clock(Math.min(s.shortSeconds, s.bufferSeconds))}</span>
        </div>
      </Row>
      <Row title="Extend instead of overlapping" desc="Saving twice in a row grows one clip, no repeated footage">
        <Switch checked={s.mergeOverlaps} onCheckedChange={(v) => set({ mergeOverlaps: v })} />
      </Row>
      <Row
        title="Keep the buffer on disk"
        desc={est ? `Frees about ${est.bufferMB.toFixed(0)} MB of RAM, hidden folder next to your clips` : "Frees RAM, hidden folder next to your clips"}
      >
        <Switch checked={s.bufferOnDisk} onCheckedChange={(v) => set({ bufferOnDisk: v })} />
      </Row>
      <Row title="Start capturing when Snag opens">
        <Switch checked={s.autoStartBuffer} onCheckedChange={(v) => set({ autoStartBuffer: v })} />
      </Row>
      <Row title="Only capture during games" desc="Pauses 45 s after you leave a game, zero cost while idle">
        <Switch checked={s.smartMode} onCheckedChange={(v) => set({ smartMode: v })} />
      </Row>
      <Row title="Sound when a clip saves" desc="Quick confirmation without leaving the game">
        <Switch checked={s.saveSound} onCheckedChange={(v) => set({ saveSound: v })} />
      </Row>
      {s.saveSound && (
        <Row title="Sound" desc={s.saveSoundName === "custom" ? <span className="break-all">{s.customSound || "No file picked"}</span> : undefined}>
          <div className="flex items-center gap-2">
            <Button size="icon" aria-label="Play sound" onClick={() => api.previewSound(s)}>
              <Play size={14} weight="fill" />
            </Button>
            <Segmented
              value={s.saveSoundName}
              onChange={async (v) => {
                if (v === "custom" && !s.customSound) {
                  const f = await api.pickSoundFile();
                  if (!f) return;
                  const next = { ...s, saveSoundName: v, customSound: f };
                  set(next);
                  api.previewSound(next);
                  return;
                }
                const next = { ...s, saveSoundName: v };
                set({ saveSoundName: v });
                api.previewSound(next);
              }}
              options={[
                { value: "pop", label: "Pop" },
                { value: "snap", label: "Snap" },
                { value: "chime", label: "Chime" },
                { value: "shutter", label: "Shutter" },
                { value: "custom", label: "Your own" },
              ]}
            />
          </div>
        </Row>
      )}
      {s.saveSound && s.saveSoundName === "custom" && (
        <Row title="Sound file" desc="A short .wav works best">
          <Button
            onClick={async () => {
              const f = await api.pickSoundFile();
              if (f) set({ customSound: f });
            }}
          >
            Choose file
          </Button>
        </Row>
      )}
      {s.saveSound && (
        <Row title="Volume">
          <div className="flex w-[260px] items-center gap-3">
            <Slider
              min={5}
              max={100}
              step={5}
              value={[s.soundVolume]}
              onValueChange={([v]) => useStore.setState({ settings: { ...s, soundVolume: v } })}
              onValueCommit={([v]) => {
                set({ soundVolume: v });
                api.previewSound({ ...s, soundVolume: v });
              }}
            />
            <span className="w-12 text-right text-base font-medium">{s.soundVolume}%</span>
          </div>
        </Row>
      )}
    </Section>
  );
}
