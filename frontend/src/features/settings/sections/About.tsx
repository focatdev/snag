import { ArrowClockwise } from "@phosphor-icons/react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { Row, Section, type SectionProps } from "../parts";

function ago(ms: number) {
  const m = Math.round((Date.now() - ms) / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  return h < 24 ? `${h} h ago` : `${Math.round(h / 24)} days ago`;
}

export function AboutSection({ s, set, version }: SectionProps & { version: string }) {
  const u = useStore((st) => st.update);
  const toast = useStore((st) => st.toast);
  let state = "Updates come with release builds from GitHub";
  if (u?.available) {
    if (u.ready) state = `Snag ${u.latest.replace(/^v/, "")} is downloaded`;
    else if (u.checking) state = "Checking";
    else if (u.error) state = u.error;
    else if (u.checkedAt) state = `Up to date, checked ${ago(u.checkedAt)}`;
    else state = s.autoUpdate ? "Checks a few times a day" : "Off";
  }
  return (
    <Section id="about" title="About">
      <Row title={`Snag ${version}`} desc={state}>
        {u?.ready ? (
          <Button variant="primary" onClick={() => api.restartToUpdate().catch((e) => toast("error", String(e)))}>
            Restart to update
          </Button>
        ) : (
          <Button
            disabled={!u?.available || u.checking}
            onClick={() =>
              api
                .checkUpdate()
                .then((n) => useStore.setState({ update: n }))
                .catch((e) => toast("error", String(e)))
            }
          >
            <ArrowClockwise size={13} className={u?.checking ? "animate-spin" : ""} />
            Check now
          </Button>
        )}
      </Row>
      <Row title="Update automatically" desc="Asks GitHub for new releases, checked against their published hash before installing">
        <Switch checked={s.autoUpdate} onCheckedChange={(v) => set({ autoUpdate: v })} />
      </Row>
      <Row title="First-run setup" desc="FFmpeg, GPU check and hotkeys, step by step">
        <Button variant="ghost" onClick={() => set({ onboarded: false })}>
          Run it again
        </Button>
      </Row>
      <Row title="Report a problem" desc="Versions, GPU, encoder results and recent log lines, never your keys">
        <div className="flex gap-2">
          <Button variant="ghost" onClick={() => api.openLogFolder()}>
            Open log
          </Button>
          <Button
            onClick={() =>
              api
                .diagnostics()
                .then((d) => api.copyText(d))
                .then(() => toast("success", "Diagnostics copied, paste them in your report"))
                .catch((e) => toast("error", String(e)))
            }
          >
            Copy diagnostics
          </Button>
        </div>
      </Row>
      <p className="mt-4 text-2xs leading-relaxed text-faint">
        Photos from Unsplash and Pexels, via the elementary OS wallpaper set. Popup font is Instrument Sans, SIL Open Font License.
      </p>
    </Section>
  );
}
