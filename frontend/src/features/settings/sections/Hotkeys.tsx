import { useStore } from "@/lib/store";
import { HotkeyInput } from "../HotkeyInput";
import { Row, Section, type SectionProps } from "../parts";

export function HotkeysSection({ s, set }: SectionProps) {
  const status = useStore((st) => st.status);
  return (
    <Section id="hotkeys" title="Hotkeys">
      {status && status.hotkeyErrors.length > 0 && (
        <p className="mt-2 rounded-[6px] border border-[#4a3d22] bg-[#221d13] px-3 py-2 text-xs text-warn">{status.hotkeyErrors.join(", ")}</p>
      )}
      <Row title="Save clip" desc="Works in fullscreen too">
        <HotkeyInput value={s.hotkeys.saveClip} onChange={(v) => set({ hotkeys: { ...s.hotkeys, saveClip: v } })} />
      </Row>
      <Row title="Save short clip" desc={`Last ${Math.min(s.shortSeconds, s.bufferSeconds)} seconds only`}>
        <HotkeyInput value={s.hotkeys.saveShort} onChange={(v) => set({ hotkeys: { ...s.hotkeys, saveShort: v } })} />
      </Row>
      <Row title="Screenshot" desc={s.copyScreenshots ? "Saved to Screenshots and copied" : "Saved to the Screenshots folder"}>
        <HotkeyInput value={s.hotkeys.screenshot} onChange={(v) => set({ hotkeys: { ...s.hotkeys, screenshot: v } })} />
      </Row>
      <Row title="Bookmark" desc="Marks the moment on your next clip">
        <HotkeyInput value={s.hotkeys.bookmark} onChange={(v) => set({ hotkeys: { ...s.hotkeys, bookmark: v } })} />
      </Row>
      <Row title="Start or stop capturing">
        <HotkeyInput value={s.hotkeys.toggleBuffer} onChange={(v) => set({ hotkeys: { ...s.hotkeys, toggleBuffer: v } })} />
      </Row>
    </Section>
  );
}
