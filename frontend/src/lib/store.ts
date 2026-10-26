import { create } from "zustand";
import { api, connect, on, type AppStatus, type Bootstrap, type Clip, type Job, type Settings, type UpdateState } from "./api";

export type LibraryKind = "all" | "clips" | "exports" | "favorites";
export type View = { name: "home" } | { name: "library"; kind?: LibraryKind; game?: string } | { name: "settings"; section?: string } | { name: "clip"; id: string };

export type Toast = { id: number; kind: "info" | "success" | "error"; message: string };

type State = {
  ready: boolean;
  fatal: string;
  boot: Bootstrap | null;
  settings: Settings | null;
  status: AppStatus | null;
  clips: Clip[];
  jobs: Job[];
  view: View;
  toasts: Toast[];
  update: UpdateState | null;
  settingsActive: string;
  shortcutsOpen: boolean;
  go: (v: View) => void;
  toast: (kind: Toast["kind"], message: string) => void;
  dismiss: (id: number) => void;
  setSettings: (s: Settings) => Promise<void>;
  refreshClips: () => Promise<void>;
  patchClip: (c: Clip) => void;
};

let toastId = 0;

export const useStore = create<State>((set, get) => ({
  ready: false,
  fatal: "",
  boot: null,
  settings: null,
  status: null,
  clips: [],
  jobs: [],
  view: { name: "home" },
  toasts: [],
  update: null,
  settingsActive: "capture",
  shortcutsOpen: false,
  go: (view) => set({ view }),
  toast: (kind, message) => {
    const id = ++toastId;
    set({ toasts: [...get().toasts.slice(-3), { id, kind, message }] });
    setTimeout(() => get().dismiss(id), kind === "error" ? 7000 : 3500);
  },
  dismiss: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),
  setSettings: async (s) => {
    set({ settings: s });
    try {
      const saved = await api.saveSettings(s);
      set({ settings: saved });
    } catch (e) {
      get().toast("error", String(e));
    }
  },
  refreshClips: async () => {
    const clips = await api.clips();
    set({ clips: clips ?? [] });
  },
  patchClip: (c) => set({ clips: get().clips.map((x) => (x.id === c.id ? c : x)) }),
}));

export async function boot() {
  const s = useStore.getState();
  try {
    const b = await connect();
    useStore.setState({ ready: true, boot: b, settings: b.settings, status: b.status, clips: b.clips ?? [], jobs: b.jobs ?? [], update: b.update ?? null });
  } catch (e) {
    useStore.setState({ fatal: String(e) });
    return;
  }
  on<AppStatus>("status", (status) => useStore.setState({ status }));
  on<Settings>("settings:changed", (settings) => useStore.setState({ settings }));
  on<Job[]>("jobs:changed", (jobs) => useStore.setState({ jobs: jobs ?? [] }));
  let pending: number | undefined;
  on("clips:changed", () => {
    // thumbnails fire a lot of these
    window.clearTimeout(pending);
    pending = window.setTimeout(() => useStore.getState().refreshClips(), 250);
  });
  on<UpdateState>("update", (update) => useStore.setState({ update }));
  // the popup covers it when snag isn't focused
  on<Clip>("clip:saved", (c) => document.hasFocus() && s.toast("success", `Saved ${c.title}`));
  on<{ kind: "info" | "success" | "error"; message: string }>("toast", (t) => s.toast(t.kind, t.message));
}

export function mediaUrl(kind: "clip" | "thumb" | "sprite" | "job", id: string, bust?: string | number) {
  const m = useStore.getState().boot?.media;
  if (!m || !m.base) return "";
  return `${m.base}/m/${kind}/${id}?t=${m.token}${bust !== undefined ? `&v=${bust}` : ""}`;
}
