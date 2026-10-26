import { FolderOpen } from "@phosphor-icons/react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { api, type Settings as S } from "@/lib/api";
import { bytes } from "@/lib/format";
import { useStore } from "@/lib/store";
import { Row, Section, type SectionProps } from "../parts";

const tokens = ["{game}", "{date}", "{time}", "{length}"];

function NameTemplate({ s, onSave }: { s: S; onSave: (t: string) => void }) {
  const [value, setValue] = useState(s.fileTemplate);
  const [preview, setPreview] = useState("");
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => setValue(s.fileTemplate), [s.fileTemplate]);
  useEffect(() => {
    const t = window.setTimeout(() => {
      api
        .previewFileName({ ...s, fileTemplate: value })
        .then(setPreview)
        .catch(() => {});
    }, 150);
    return () => window.clearTimeout(t);
  }, [value, s.organizeByGame, s.bufferSeconds]);
  const commit = () => {
    if (value.trim() && value !== s.fileTemplate) onSave(value.trim());
    if (!value.trim()) setValue(s.fileTemplate);
  };
  const insert = (tok: string) => {
    const el = input.current;
    const at = el?.selectionStart ?? value.length;
    const next = value.slice(0, at) + tok + value.slice(el?.selectionEnd ?? at);
    setValue(next);
    onSave(next);
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(at + tok.length, at + tok.length);
    });
  };
  return (
    <Row title="File name" desc="Click a tag to add it" wide>
      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <Input
            ref={input}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onBlur={commit}
            onKeyDown={(e) => e.key === "Enter" && commit()}
            className="flex-1"
            spellCheck={false}
          />
          {value !== "{game} {date} {time}" && (
            <Button variant="ghost" onClick={() => (setValue("{game} {date} {time}"), onSave("{game} {date} {time}"))}>
              Reset
            </Button>
          )}
        </div>
        <div className="flex items-center gap-1.5">
          {tokens.map((t) => (
            <button key={t} onClick={() => insert(t)} className="rounded-[4px] border border-line px-1.5 py-0.5 text-2xs text-muted hover:border-[#3a3e44] hover:text-fg">
              {t}
            </button>
          ))}
          <span className="ml-auto truncate text-xs text-faint" title={preview}>
            Next clip: <span className="text-muted">{preview}</span>
          </span>
        </div>
      </div>
    </Row>
  );
}

export function StorageSection({ s, set }: SectionProps) {
  const usage = useStore((st) => st.status?.usage);
  const budget = s.storageBudgetGB * 1024 ** 3;
  return (
    <Section id="storage" title="Storage">
      <Row title="Clips folder" desc={<span className="break-all">{s.clipsDir}</span>}>
        <div className="flex gap-2">
          <Button size="icon" aria-label="Open folder" onClick={() => api.openClipsFolder()}>
            <FolderOpen size={16} />
          </Button>
          <Button
            onClick={async () => {
              const p = await api.pickFolder(s.clipsDir);
              if (p) set({ clipsDir: p });
            }}
          >
            Change
          </Button>
        </div>
      </Row>
      <Row title="Exports folder" desc={<span className="break-all">{s.exportsDir || `${s.clipsDir}\\Exports`}</span>}>
        <div className="flex gap-2">
          {s.exportsDir && (
            <Button variant="ghost" onClick={() => set({ exportsDir: "" })}>
              Reset
            </Button>
          )}
          <Button
            onClick={async () => {
              const p = await api.pickFolder(s.exportsDir || s.clipsDir);
              if (p) set({ exportsDir: p });
            }}
          >
            Change
          </Button>
        </div>
      </Row>
      <Row title="Copy exports to clipboard" desc="Ready to paste in Discord">
        <Switch checked={s.copyExports} onCheckedChange={(v) => set({ copyExports: v })} />
      </Row>
      <Row title="Copy screenshots to clipboard">
        <Switch checked={s.copyScreenshots} onCheckedChange={(v) => set({ copyScreenshots: v })} />
      </Row>
      <Row title="Shrink old clips" desc="Re-encodes to AV1 when your PC is idle, about half the size">
        <Select
          value={String(s.autoShrinkDays)}
          onChange={(v) => set({ autoShrinkDays: Number(v) })}
          options={[
            { value: "0", label: "Never" },
            { value: "3", label: "After 3 days" },
            { value: "7", label: "After a week" },
            { value: "14", label: "After 2 weeks" },
            { value: "30", label: "After a month" },
          ]}
        />
      </Row>
      <Row title="Folder per game" desc="Like Snag\Roblox, library still shows all">
        <Switch checked={s.organizeByGame} onCheckedChange={(v) => set({ organizeByGame: v })} />
      </Row>
      <NameTemplate s={s} onSave={(fileTemplate) => set({ fileTemplate })} />
      <Row title="Storage limit" desc="Deletes oldest clips first, never favorites">
        <Switch checked={s.storageBudgetEnabled} onCheckedChange={(v) => set({ storageBudgetEnabled: v })} />
      </Row>
      {s.storageBudgetEnabled && (
        <Row title="Limit" wide>
          <div className="flex items-center gap-4">
            <Slider
              min={2}
              max={200}
              step={1}
              value={[s.storageBudgetGB]}
              onValueChange={([v]) => useStore.setState({ settings: { ...s, storageBudgetGB: v } })}
              onValueCommit={([v]) => set({ storageBudgetGB: v })}
            />
            <span className="w-16 text-right text-base font-medium">{s.storageBudgetGB} GB</span>
          </div>
        </Row>
      )}
      {usage && (
        <div className="mt-2 rounded-card border border-line-soft p-4">
          <div className="mb-2 flex justify-between text-xs">
            <span className="text-fg">
              {bytes(usage.usedBytes)} in {usage.clipCount} clips
            </span>
            <span className="text-muted">{bytes(usage.freeBytes, 0)} free on this drive</span>
          </div>
          <div className="relative h-2 overflow-hidden rounded-full bg-line">
            <div
              className="absolute inset-y-0 left-0 bg-frost"
              style={{ width: `${Math.min(100, (usage.usedBytes / (s.storageBudgetEnabled ? budget : usage.totalBytes || 1)) * 100)}%` }}
            />
          </div>
          <p className="mt-2 text-2xs text-faint">{s.storageBudgetEnabled ? `Compared to your ${s.storageBudgetGB} GB limit` : "Compared to the whole drive"}</p>
        </div>
      )}
    </Section>
  );
}
