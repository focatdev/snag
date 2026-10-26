import { ArrowSquareOut, Trash } from "@phosphor-icons/react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Tip } from "@/components/ui/tooltip";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { api, openUrl, type Uploader } from "@/lib/api";
import { useStore } from "@/lib/store";
import { Row, Section, type SectionProps } from "../parts";

export function SharingSection({ s, set }: SectionProps) {
  const [list, setList] = useState<Uploader[]>([]);
  const [paste, setPaste] = useState(false);
  const [text, setText] = useState("");
  const refresh = () =>
    api
      .listUploaders()
      .then((l) => setList(l ?? []))
      .catch(() => {});
  useEffect(() => {
    refresh();
  }, []);
  const current = list.find((u) => u.id === (s.uploader || "catbox"));
  const isCatbox = !current || current.builtin;
  const imported = (u: Uploader | null) => {
    if (!u) return;
    refresh();
    set({ uploader: u.id });
    useStore.getState().toast("success", `${u.name} added`);
  };
  const fail = (e: unknown) => useStore.getState().toast("error", String(e));

  return (
    <Section id="sharing" title="Sharing">
      <p className="mb-1 mt-1 text-xs text-muted">Nothing uploads unless you ask, or turn on uploading new clips below. Links are public to anyone who has them.</p>
      <Row
        title="Upload to"
        desc={
          isCatbox ? (
            <>
              <button className="underline underline-offset-2 hover:text-fg" onClick={() => openUrl("https://catbox.moe")}>
                catbox.moe
              </button>
              , free, 200 MB per file
            </>
          ) : current && !current.videoReady ? (
            <span className="text-warn">Set up for images only, it may turn videos away</span>
          ) : (
            current?.host
          )
        }
      >
        <div className="flex items-center gap-2">
          {!isCatbox && current && (
            <Tip label={`Remove ${current.name}`}>
              <Button variant="ghost" size="icon" onClick={() => api.deleteUploader(current.id).then(refresh).catch(fail)}>
                <Trash size={15} />
              </Button>
            </Tip>
          )}
          <Select
            className="min-w-[200px]"
            value={current?.id ?? "catbox"}
            onChange={(v) => set({ uploader: v })}
            options={list.map((u) => ({ value: u.id, label: u.name, hint: u.builtin ? undefined : u.host }))}
          />
        </div>
      </Row>
      <Row title="Custom uploader" desc="Any ShareX .sxcu works, the same file people share for ShareX">
        <div className="flex gap-2">
          <Button variant="ghost" onClick={() => setPaste(true)}>
            Paste
          </Button>
          <Button onClick={() => api.importUploader().then(imported).catch(fail)}>Import .sxcu</Button>
        </div>
      </Row>
      <Row title="Upload new clips" desc="Link lands on your clipboard when it's done">
        <Switch checked={s.catboxAutoUpload} onCheckedChange={(v) => set({ catboxAutoUpload: v })} />
      </Row>
      <Row title="Delete after uploading" desc="Recycles the local file once the link is copied, never favorites">
        <Switch checked={s.catboxDeleteAfter} onCheckedChange={(v) => set({ catboxDeleteAfter: v })} />
      </Row>
      {isCatbox && <UserhashRow value={s.catboxUserhash} onSave={(catboxUserhash) => set({ catboxUserhash })} />}

      <Dialog open={paste} onOpenChange={setPaste} title="Paste a custom uploader" description="The contents of a .sxcu file, as JSON." className="w-[520px]">
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          spellCheck={false}
          placeholder='{ "Name": "My host", "RequestURL": "https://...", ... }'
          className="mt-2 h-[220px] w-full resize-none rounded-[6px] border border-line bg-well p-3 font-mono text-xs text-fg outline-none placeholder:text-faint focus:border-[#4a5260]"
        />
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setPaste(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            disabled={!text.trim()}
            onClick={() =>
              api
                .importUploaderText(text)
                .then((u) => {
                  imported(u);
                  setPaste(false);
                  setText("");
                })
                .catch(fail)
            }
          >
            Add uploader
          </Button>
        </div>
      </Dialog>
    </Section>
  );
}

function UserhashRow({ value, onSave }: { value: string; onSave: (v: string) => void }) {
  const [v, setV] = useState(value);
  useEffect(() => setV(value), [value]);
  const commit = () => v.trim() !== value && onSave(v.trim());
  return (
    <Row
      title="Catbox account"
      desc={
        <>
          Optional userhash, so you can delete uploads later.{" "}
          <button className="inline-flex items-center gap-1 underline underline-offset-2 hover:text-fg" onClick={() => openUrl("https://catbox.moe/")}>
            It's on your catbox account page <ArrowSquareOut size={11} />
          </button>
        </>
      }
    >
      <Input
        type="password"
        value={v}
        placeholder="Anonymous"
        onChange={(e) => setV(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => e.key === "Enter" && commit()}
        className="w-[240px]"
        spellCheck={false}
        autoComplete="off"
      />
    </Row>
  );
}
