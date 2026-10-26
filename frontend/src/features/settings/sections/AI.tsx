import { ArrowSquareOut } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { api, openUrl, type AIProvider, type Settings as S } from "@/lib/api";
import { useStore } from "@/lib/store";
import { Row, Section, type SectionProps } from "../parts";

export function AISection({ s, set }: SectionProps) {
  const [providers, setProviders] = useState<AIProvider[]>([]);
  const [key, setKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [model, setModel] = useState(s.aiModel);
  useEffect(() => {
    api
      .aiProviders()
      .then(setProviders)
      .catch(() => {});
  }, []);
  useEffect(() => setModel(s.aiModel), [s.aiModel]);
  const p = providers.find((x) => x.id === s.aiProvider);
  const saved = !!s.aiKey;

  const saveKey = async () => {
    if (!key.trim()) return;
    setSaving(true);
    try {
      await api.setAIKey(key.trim());
      useStore.setState({ settings: { ...s, aiKey: "saved" } });
      setKey("");
      useStore.getState().toast("success", "Key saved, encrypted to your Windows account");
    } catch (e) {
      useStore.getState().toast("error", String(e));
    }
    setSaving(false);
  };

  return (
    <Section id="ai" title="AI">
      <p className="mb-1 mt-1 text-xs text-muted">Optional. Uses your own key, billed by the provider. Nothing leaves your PC until you pick one.</p>
      <Row title="Provider">
        <Select
          className="min-w-[200px]"
          value={s.aiProvider || "none"}
          onChange={(v) => set({ aiProvider: (v === "none" ? "" : v) as S["aiProvider"], aiModel: "", aiKey: "" })}
          options={[{ value: "none", label: "Off" }, ...providers.map((x) => ({ value: x.id, label: x.label }))]}
        />
      </Row>
      {p && (
        <>
          <Row
            title="API key"
            desc={
              <button className="inline-flex items-center gap-1 underline underline-offset-2 hover:text-fg" onClick={() => openUrl(p.keyUrl)}>
                Get a {p.label} key <ArrowSquareOut size={11} />
              </button>
            }
          >
            <div className="flex gap-2">
              <Input
                type="password"
                value={key}
                placeholder={saved ? "Saved. Paste to replace" : "Paste key"}
                onChange={(e) => setKey(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && saveKey()}
                className="w-[240px]"
                spellCheck={false}
                autoComplete="off"
              />
              <Button disabled={!key.trim() || saving} onClick={saveKey}>
                Save
              </Button>
            </div>
          </Row>
          <Row title="Model" desc="Leave empty for the default">
            <Input
              value={model}
              placeholder={p.model}
              onChange={(e) => setModel(e.target.value)}
              onBlur={() => model.trim() !== s.aiModel && set({ aiModel: model.trim() })}
              onKeyDown={(e) => e.key === "Enter" && model.trim() !== s.aiModel && set({ aiModel: model.trim() })}
              className="w-[240px]"
              spellCheck={false}
            />
          </Row>
          <Row title="Name new clips" desc="3 frames per clip, a fraction of a cent">
            <Switch disabled={!saved} checked={s.aiAutoTitle} onCheckedChange={(v) => set({ aiAutoTitle: v })} />
          </Row>
          <Row title="Search by voice" desc={p.transcribe ? "Turns what you said into searchable text" : `${p.label} can't hear audio, pick another provider`}>
            <Switch disabled={!saved || !p.transcribe} checked={s.aiTranscribe && p.transcribe} onCheckedChange={(v) => set({ aiTranscribe: v })} />
          </Row>
          <p className="mt-3 text-2xs leading-relaxed text-faint">
            Frames and voice audio go straight from your PC to {p.label}. Turn both off and nothing gets sent unless you click Name with AI.
          </p>
        </>
      )}
    </Section>
  );
}
