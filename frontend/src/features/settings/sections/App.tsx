import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { api, type Settings as S } from "@/lib/api";
import { Row, Section, type SectionProps } from "../parts";

export function AppSection({ s, set }: SectionProps) {
  return (
    <Section id="app" title="App">
      <Row title="Popup when a clip saves" desc="Never shows up in recordings">
        <Switch checked={s.showOverlay} onCheckedChange={(v) => set({ showOverlay: v })} />
      </Row>
      {s.showOverlay && (
        <Row title="Popup position">
          <div className="flex gap-2">
            <Button variant="ghost" onClick={() => api.testOverlay()}>
              Show test
            </Button>
            <Select
              value={s.overlayCorner}
              onChange={(v) => set({ overlayCorner: v as S["overlayCorner"] })}
              options={[
                { value: "top-right", label: "Top right" },
                { value: "top-left", label: "Top left" },
                { value: "bottom-right", label: "Bottom right" },
                { value: "bottom-left", label: "Bottom left" },
              ]}
            />
          </div>
        </Row>
      )}
      <Row title="Start with Windows" desc="Opens quietly in the tray">
        <Switch checked={s.launchOnStartup} onCheckedChange={(v) => set({ launchOnStartup: v })} />
      </Row>
      <Row title="Start in the tray" desc="No window on launch">
        <Switch checked={s.startMinimized} onCheckedChange={(v) => set({ startMinimized: v })} />
      </Row>
      <Row title="Keep running when closed" desc="Hotkeys keep working from the tray">
        <Switch checked={s.closeToTray} onCheckedChange={(v) => set({ closeToTray: v })} />
      </Row>
    </Section>
  );
}
