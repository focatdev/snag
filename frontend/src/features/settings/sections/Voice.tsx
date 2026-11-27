import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { DraftInput, Row, Section, type SectionProps } from "../parts";

export function VoiceSection({ s, set }: SectionProps) {
  const voice = useStore((st) => st.status?.voice);
  const [sens, setSens] = useState(s.voiceSensitivity);
  useEffect(() => setSens(s.voiceSensitivity), [s.voiceSensitivity]);

  const dl = voice?.download;
  const pct = dl?.total ? Math.round((dl.done / dl.total) * 100) : 0;
  const dlLabel = dl?.active ? `Downloading ${pct}%` : voice?.ready ? "Download again" : "Download";

  return (
    <Section id="voice" title="Voice">
      <p className="mb-1 mt-1 text-xs text-muted">
        Say a phrase to save a clip. Only listens while capturing. Uses the mic
        from Audio.
      </p>
      <Row
        title="Voice trigger"
        desc="Listens only for the phrase, light on the CPU"
      >
        <Switch
          checked={s.voiceTrigger}
          onCheckedChange={(v) => set({ voiceTrigger: v })}
        />
      </Row>
      {s.voiceTrigger && (
        <>
          <Row title="Phrase" desc="English words, saves a full-length clip">
            <DraftInput
              value={s.voicePhrase}
              onCommit={(v) => set({ voicePhrase: v })}
              className="w-[320px]"
            />
          </Row>
          <Row
            title="Sensitivity"
            desc="Higher catches mumbled phrases, lower avoids false saves"
          >
            <div className="flex w-[320px] items-center gap-3">
              <Slider
                min={0}
                max={100}
                step={5}
                value={[sens]}
                onValueChange={([v]) => setSens(v)}
                onValueCommit={([v]) => set({ voiceSensitivity: v })}
              />
              <span className="w-8 text-right text-xs tabular-nums text-muted">
                {sens}
              </span>
            </div>
          </Row>
          <Row
            title={
              voice?.ready
                ? "Keyword spotter ready"
                : "Keyword spotter not installed"
            }
            desc="sherpa-onnx with a small English model, about a 25 MB download"
          >
            <Button
              onClick={() =>
                api
                  .downloadVoice()
                  .catch((e) => useStore.getState().toast("error", String(e)))
              }
              disabled={dl?.active}
            >
              {dlLabel}
            </Button>
          </Row>
          <Row
            title="Install folder"
            desc={<span className="break-all">{voice?.dir}</span>}
          >
            <div className="flex gap-2">
              {s.voiceDir && (
                <Button variant="ghost" onClick={() => set({ voiceDir: "" })}>
                  Reset
                </Button>
              )}
              <Button
                onClick={async () => {
                  const p = await api.pickFolder(s.voiceDir || "");
                  if (p) set({ voiceDir: p });
                }}
              >
                Change
              </Button>
            </div>
          </Row>
          {voice?.error && <p className="text-2xs text-bad">{voice.error}</p>}
          {dl?.error && !dl.active && (
            <p className="text-2xs text-bad">{dl.error}</p>
          )}
          <p className="mt-3 text-2xs leading-relaxed text-faint">
            Runs on your PC, nothing is sent anywhere.
          </p>
        </>
      )}
    </Section>
  );
}
