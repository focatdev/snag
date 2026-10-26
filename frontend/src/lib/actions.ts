import { api } from "./api";
import { useStore } from "./store";

const toast = (kind: "success" | "error" | "info", msg: string) => useStore.getState().toast(kind, msg);

export async function shrink(ids: string[]) {
  try {
    const n = await api.shrinkClips(ids);
    if (n === 0) toast("info", ids.length === 1 ? "Already as small as it gets" : "Those are already shrunk");
    else toast("success", n === 1 ? "Shrinking in the background" : `Shrinking ${n} clips in the background`);
  } catch (e) {
    toast("error", String(e));
  }
}

export async function nameWithAI(id: string) {
  try {
    const c = await api.describeClip(id);
    useStore.getState().patchClip(c);
    toast("success", `Named it "${c.title}"`);
  } catch (e) {
    toast("error", String(e));
  }
}

export async function transcribe(id: string) {
  try {
    const c = await api.transcribeClip(id);
    useStore.getState().patchClip(c);
    toast("success", c.transcript ? "Voice added to search" : "Nobody talks in this one");
    return c;
  } catch (e) {
    toast("error", String(e));
  }
}

export async function uploadClip(id: string) {
  try {
    await api.uploadClip(id);
    toast("info", "Uploading, the link gets copied when it's done");
  } catch (e) {
    toast("error", String(e));
  }
}

export async function copyLink(url: string) {
  try {
    await api.copyText(url);
    toast("success", "Link copied");
  } catch (e) {
    toast("error", String(e));
  }
}

export const aiReady = () => {
  const s = useStore.getState().settings;
  return !!s?.aiProvider && !!s.aiKey;
};
