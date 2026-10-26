import { useEffect, useState } from "react";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { api, type AudioDevice } from "@/lib/api";
import { Row, Section, type SectionProps } from "../parts";

export function AudioSection({ s, set }: SectionProps) {
  const [devices, setDevices] = useState<{ outputs: AudioDevice[]; inputs: AudioDevice[] }>({ outputs: [], inputs: [] });
  useEffect(() => {
    api
      .audioDevices()
      .then(setDevices)
      .catch(() => {});
  }, []);
  return (
    <Section id="audio" title="Audio">
      <Row title="Game and desktop audio" desc="Everything you hear, on its own track">
        <Switch checked={s.captureSystemAudio} onCheckedChange={(v) => set({ captureSystemAudio: v })} />
      </Row>
      {s.captureSystemAudio && (
        <Row title="Discord on its own track" desc="Mute friends later without losing game sound. Windows 11">
          <Switch checked={s.separateVoice} onCheckedChange={(v) => set({ separateVoice: v })} />
        </Row>
      )}
      {s.captureSystemAudio && (
        <Row title="Output device">
          <Select
            className="min-w-[260px]"
            value={s.systemDeviceId || "default"}
            onChange={(v) => set({ systemDeviceId: v === "default" ? "" : v })}
            options={[{ value: "default", label: "Windows default" }, ...devices.outputs.map((d) => ({ value: d.id, label: d.name }))]}
          />
        </Row>
      )}
      <Row title="Microphone" desc="Own track in mono, half the size">
        <Switch checked={s.captureMic} onCheckedChange={(v) => set({ captureMic: v })} />
      </Row>
      {s.captureMic && (
        <Row title="Skip the mic when you don't talk" desc="Silent mic tracks get left out of the clip">
          <Switch checked={s.dropSilentMic} onCheckedChange={(v) => set({ dropSilentMic: v })} />
        </Row>
      )}
      {s.captureMic && (
        <Row title="Input device">
          <Select
            className="min-w-[260px]"
            value={s.micDeviceId || "default"}
            onChange={(v) => set({ micDeviceId: v === "default" ? "" : v })}
            options={[{ value: "default", label: "Windows default" }, ...devices.inputs.map((d) => ({ value: d.id, label: d.name }))]}
          />
        </Row>
      )}
    </Section>
  );
}
