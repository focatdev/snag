import { Call, Events, Window as WailsWindow } from "@wailsio/runtime";

export type Drive = { root: string; label: string; free: number; total: number; system: boolean; removable: boolean };
export type Paths = { app: string; clips: string; ffmpeg: string };
export type Info = {
  version: string;
  installed: string;
  autostart: boolean;
  uninstall: boolean;
  update: boolean;
  drives: Drive[];
  home: string;
  localAppData: string;
  existing: Paths;
  payloadBytes: number;
  clipCount: number;
  clipBytes: number;
};
export type Options = { paths: Paths; downloadFFmpeg: boolean; startWithWindows: boolean; desktopShortcut: boolean; startMenu: boolean };
export type Progress = { step: string; pct: number; note: string; done: boolean; error: string };

const params = new URLSearchParams(location.search);
const mock = params.has("mock");
const listeners = new Set<(p: Progress) => void>();

function fakeRun(steps: [string, string][]) {
  let i = 0;
  const t = setInterval(() => {
    const pct = Math.min(1, (i + 1) / 28);
    const [step, note] = steps[Math.min(steps.length - 1, Math.floor(pct * steps.length))];
    listeners.forEach((l) => l({ step: pct >= 1 ? "done" : step, pct, note, done: pct >= 1, error: "" }));
    if (pct >= 1) clearInterval(t);
    i++;
  }, 180);
}

const mockInfo: Info = {
  version: "0.2.0",
  installed: "0.1.0",
  autostart: true,
  uninstall: params.has("uninstall"),
  update: params.has("update"),
  drives: [
    { root: "C:\\", label: "Windows", free: 3.1 * 1024 ** 3, total: 237 * 1024 ** 3, system: true, removable: false },
    { root: "D:\\", label: "Games", free: 412 * 1024 ** 3, total: 931 * 1024 ** 3, system: false, removable: false },
    { root: "E:\\", label: "Backup", free: 88 * 1024 ** 3, total: 465 * 1024 ** 3, system: false, removable: true },
  ],
  home: "C:\\Users\\you",
  localAppData: "C:\\Users\\you\\AppData\\Local",
  existing: { app: "", clips: "", ffmpeg: "" },
  payloadBytes: 13 * 1024 * 1024,
  clipCount: 42,
  clipBytes: 3.4 * 1024 ** 3,
};

const call = <T,>(m: string, ...a: unknown[]) => Call.ByName(`main.SetupService.${m}`, ...a) as Promise<T>;

export const setup = {
  info: () => (mock ? Promise.resolve(mockInfo) : call<Info>("Info")),
  pickFolder: (start: string) => (mock ? Promise.resolve(start) : call<string>("PickFolder", start)),
  install: (o: Options) =>
    mock
      ? (fakeRun([
          ["files", "Copying Snag"],
          ["settings", "Saving your folders"],
          ["shortcuts", "Adding shortcuts"],
          ["ffmpeg", "Downloading FFmpeg, 41 of 98 MB"],
          ["ffmpeg", "Downloading FFmpeg, 87 of 98 MB"],
          ["register", "Registering with Windows"],
        ]),
        Promise.resolve())
      : call<void>("Install", o),
  uninstall: (o: { removeClips: boolean; removeData: boolean }) =>
    mock
      ? (fakeRun([
          ["files", "Closing Snag"],
          ["shortcuts", "Removing shortcuts"],
          ["register", "Removing Snag"],
        ]),
        Promise.resolve())
      : call<void>("Uninstall", o),
  launch: () => (mock ? Promise.resolve() : call<void>("Launch")),
  quit: () => (mock ? Promise.resolve() : call<void>("Quit")),
  onProgress: (cb: (p: Progress) => void) => {
    if (mock) {
      listeners.add(cb);
      return () => listeners.delete(cb);
    }
    return Events.On("setup:progress", (e: { data: unknown }) => cb(e.data as Progress));
  },
  minimise: () => (mock ? undefined : WailsWindow.Minimise()),
};
