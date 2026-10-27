import { FolderSimple, FolderSimpleMinus, FolderSimplePlus } from "@phosphor-icons/react";
import { useEffect, useRef, useState, type DragEvent } from "react";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import type { MenuEntry } from "@/components/ui/menu";
import { api, type ClipPatch, type Folder } from "@/lib/api";
import { useStore, type FolderDialog as DialogState } from "@/lib/store";

export const CLIP_DRAG = "application/x-snag-clips";

const toast = (kind: "success" | "error" | "info", msg: string) => useStore.getState().toast(kind, msg);
const clipsLabel = (n: number) => (n === 1 ? "1 clip" : `${n} clips`);

export const openFolderDialog = (d: DialogState) => useStore.setState({ folderDialog: d });

// patch the store first so the grid doesn't wait on the debounced refresh
export async function patchClips(ids: string[], patch: ClipPatch) {
  const set = new Set(ids);
  useStore.setState((s) => ({ clips: s.clips.map((c) => (set.has(c.id) ? { ...c, ...patch } : c)) }));
  try {
    await api.updateClips(ids, patch);
    return true;
  } catch (e) {
    toast("error", String(e));
    useStore.getState().refreshClips();
    return false;
  }
}

export async function moveToFolder(ids: string[], folder: Folder | null) {
  if (!(await patchClips(ids, { folder: folder?.id ?? "" }))) return false;
  if (folder) toast("success", `Moved ${ids.length === 1 ? "" : clipsLabel(ids.length) + " "}to ${folder.name}`);
  else toast("success", ids.length === 1 ? "Taken out of the folder" : `Took ${clipsLabel(ids.length)} out of their folders`);
  return true;
}

// current is where the clips already are, so it isn't offered as a target
export function folderMenu(folders: Folder[], ids: string[], current: (string | undefined)[], onDone?: () => void): MenuEntry[] {
  const inside = new Set(current.filter(Boolean));
  const targets = folders.filter((f) => !(inside.size === 1 && inside.has(f.id)));
  const known = folders.some((f) => inside.has(f.id));
  const done = (ok: boolean) => ok && onDone?.();
  return [
    ...targets.map((f) => ({ label: f.name, icon: <FolderSimple size={15} />, onSelect: () => moveToFolder(ids, f).then(done) })),
    ...(targets.length ? ["sep" as const] : []),
    { label: "New folder", icon: <FolderSimplePlus size={15} />, onSelect: () => openFolderDialog({ mode: "new", ids, onDone }) },
    ...(known ? [{ label: "Take out of folder", icon: <FolderSimpleMinus size={15} />, onSelect: () => moveToFolder(ids, null).then(done) }] : []),
  ];
}

export function dragIds(e: DragEvent): string[] | null {
  if (!e.dataTransfer.types.includes(CLIP_DRAG)) return null;
  try {
    const ids = JSON.parse(e.dataTransfer.getData(CLIP_DRAG));
    return Array.isArray(ids) ? ids : null;
  } catch {
    return null;
  }
}

export function FolderDialog() {
  const d = useStore((s) => s.folderDialog);
  const clips = useStore((s) => s.clips);
  const [name, setName] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setName(d?.mode === "rename" ? d.folder.name : "");
    setErr("");
    setBusy(false);
  }, [d]);

  if (!d) return null;
  const close = () => useStore.setState({ folderDialog: null });

  if (d.mode === "delete") {
    const n = clips.filter((c) => c.folder === d.folder.id).length;
    return (
      <Dialog
        open
        onOpenChange={(o) => !o && close()}
        title={`Delete ${d.folder.name}?`}
        description={n ? `The ${clipsLabel(n)} in it stay in your library, only the folder goes.` : "It's empty, nothing else changes."}
      >
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="ghost" onClick={close}>
            Keep it
          </Button>
          <Button
            variant="danger"
            onClick={async () => {
              try {
                await api.deleteFolder(d.folder.id);
                const v = useStore.getState().view;
                if (v.name === "library" && v.folder === d.folder.id) useStore.getState().go({ name: "library", kind: "clips" });
                close();
              } catch (e) {
                toast("error", String(e));
              }
            }}
          >
            Delete folder
          </Button>
        </div>
      </Dialog>
    );
  }

  const ids = d.mode === "new" ? (d.ids ?? []) : [];
  const submit = async () => {
    setBusy(true);
    setErr("");
    try {
      if (d.mode === "rename") {
        await api.renameFolder(d.folder.id, name);
      } else {
        const f = await api.createFolder(name);
        if (ids.length && (await moveToFolder(ids, f))) d.onDone?.();
        if (!ids.length) useStore.getState().go({ name: "library", kind: "all", folder: f.id });
      }
      close();
    } catch (e) {
      // go errors start lowercase
      const msg = String(e).replace(/^Error:\s*/, "");
      setErr(msg.charAt(0).toUpperCase() + msg.slice(1));
      setBusy(false);
    }
  };

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && close()}
      initialFocus={input}
      title={d.mode === "rename" ? "Rename folder" : "New folder"}
      description={ids.length ? `The ${clipsLabel(ids.length)} you picked go in it. Files stay where they are on disk.` : "Only Snag sees folders, your files stay where they are on disk."}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (!busy) submit();
        }}
      >
        <Input ref={input} className="mt-1 w-full" value={name} maxLength={60} placeholder="Best of" onChange={(e) => setName(e.target.value)} />
        {err && <p className="mt-2 text-sm text-bad">{err}</p>}
        <div className="mt-5 flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={close}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {d.mode === "rename" ? "Rename" : "Create"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
