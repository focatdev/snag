// fake backend for ?mock
import type { AppStatus, Bootstrap, Clip, Folder, Job, Settings } from "./api";

const q = new URLSearchParams(typeof location !== "undefined" ? location.search : "");
// screenshot setups: ?mock&onboard, &update, &whatsnew, &nogpu, &noffmpeg, &waiting, &state=error, &readme
const scenario = {
  onboard: q.has("onboard"),
  update: q.has("update"),
  whatsNew: q.has("whatsnew"),
  noGpu: q.has("nogpu"),
  noFFmpeg: q.has("noffmpeg"),
  waiting: q.has("waiting"),
  state: (q.get("state") ?? "buffering") as "buffering" | "idle" | "error",
  readme: q.has("readme"),
};
const nvErr = "NVIDIA driver too old, needs 610.00 or newer";

type Listener = (d: unknown) => void;
const listeners = new Map<string, Set<Listener>>();
const emit = (name: string, data: unknown) => listeners.get(name)?.forEach((l) => l(data));

const now = Date.now();
// readme shots use free games so the images can live in a public repo
const readmeGames = ["SuperTuxKart", "Red Eclipse", "osu!", "Luanti", "OpenArena", "osu!", "SuperTuxKart", "Red Eclipse", "Luanti"];
const readmeTitles = ["perfect start boost", "chaos in the middle of bath", "first full combo on this map", "apple forest by the lake", "took the lead on dm4", "7k mania 98%", "side by side into turn 2", "futuresport montage", "canyon walk"];
const games = scenario.readme ? readmeGames : ["Roblox", "Valorant", "Counter-Strike 2", "Roblox", "Rocket League", "Desktop", "Valorant", "Roblox", "Marvel Rivals"];
const liveGame = scenario.readme ? readmeGames[0] : "Roblox";

// playwright serves these when taking readme shots
export const mockThumb = (c: { id: string }) => (scenario.readme ? `/readme/${c.id}.jpg` : null);
const clips: Clip[] = games.map((g, i) => ({
  id: `mock${i}`,
  path: `C:\\Users\\you\\Videos\\Snag\\${g} 2026-10-${24 - i} 19-3${i}-12.mp4`,
  title: scenario.readme ? readmeTitles[i] : i === 1 ? "1v4 retake on B" : i === 0 ? "obby speedrun pb" : `${g} 2026-10-${24 - i} 19-3${i}-12`,
  game: g,
  createdAt: new Date(now - i * 1000 * 60 * 60 * (5 + i * 7)).toISOString(),
  duration: [60, 60, 45.4, 60, 30.2, 60, 18.7, 60, 60][i],
  width: 2560,
  height: 1440,
  fps: 60,
  size: [92, 88, 64, 90, 41, 77, 25, 91, 86][i] * 1024 * 1024,
  codec: "h264",
  tracks: i % 3 === 0 ? ["Game", "Mic"] : ["Game"],
  markers: i === 1 ? [41.2, 47.9] : [],
  favorite: i === 1 || i === 4,
  tags: [],
  thumb: false,
  sprite: false,
  spriteFrames: 10,
  export: i === 7,
  probed: true,
  shrunk: i === 5,
  origSize: i === 5 ? 160 * 1024 * 1024 : 0,
  aiTitled: i < 2,
  transcript: i === 1 ? "no way he's still alive, I got the last two on B" : "",
  folder: i === 1 || i === 6 ? "f1" : i === 0 || i === 3 ? "f2" : undefined,
}));

let folders: Folder[] = [
  { id: "f1", name: "Best of", createdAt: new Date(now - 86400000 * 3).toISOString() },
  { id: "f2", name: "With friends", createdAt: new Date(now - 86400000).toISOString() },
];
const sortedFolders = () => [...folders].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" }));
const folderName = (name: string, except = "") => {
  const n = name.trim().replace(/\s+/g, " ");
  if (!n) throw new Error("give the folder a name");
  const taken = folders.find((f) => f.id !== except && f.name.toLowerCase() === n.toLowerCase());
  if (taken) throw new Error(`you already have a folder called ${taken.name}`);
  return n;
};

let settings: Settings = {
  clipsDir: "C:\\Users\\you\\Videos\\Snag",
  exportsDir: "",
  organizeByGame: false,
  fileTemplate: "{game} {date} {time}",
  ffmpegPath: "",
  ffmpegDir: "",
  encoder: "auto",
  codec: "h264",
  fps: 60,
  resolution: "native",
  quality: "medium",
  monitor: 0,
  drawCursor: true,
  bufferSeconds: 60,
  shortSeconds: 15,
  captureSystemAudio: true,
  captureMic: false,
  systemDeviceId: "",
  micDeviceId: "",
  hotkeys: { saveClip: "F8", saveShort: "Shift+F8", screenshot: "Ctrl+F8", bookmark: "Alt+F8", toggleBuffer: "Ctrl+Alt+F8" },
  separateVoice: false,
  dropSilentMic: true,
  bufferOnDisk: false,
  mergeOverlaps: true,
  autoShrinkDays: 0,
  copyScreenshots: true,
  catboxAutoUpload: false,
  catboxDeleteAfter: false,
  catboxUserhash: "",
  uploader: "catbox",
  aiProvider: "",
  aiKey: "",
  aiModel: "",
  aiAutoTitle: true,
  aiTranscribe: false,
  smartMode: false,
  autoStartBuffer: true,
  saveSound: true,
  saveSoundName: "pop",
  soundVolume: 70,
  customSound: "",
  showOverlay: true,
  overlayCorner: "top-right",
  copyExports: true,
  storageBudgetEnabled: false,
  storageBudgetGB: 20,
  launchOnStartup: false,
  startMinimized: false,
  closeToTray: true,
  autoUpdate: true,
  onboarded: !scenario.onboard,
  lastVersion: "0.1.0",
};

const started = Date.now() - 41_000;
const jobs: Job[] = [];

function status(): AppStatus {
  const buffered = Math.min(60, (Date.now() - started) / 1000);
  return {
    recorder: {
      state: scenario.state,
      error: scenario.state === "error" ? "ddagrab capture failed: Could not open desktop duplication output" : "",
      warning: "",
      encoder: "h264_nvenc",
      encoderLabel: "NVENC H.264",
      pipeline: "gpu",
      width: 2560,
      height: 1440,
      fps: 60,
      kbps: 12400,
      bufferedSec: buffered,
      bufferTarget: settings.bufferSeconds,
      bufferBytes: buffered * 1.6 * 1024 * 1024,
      tracks: ["Game"],
      startedAt: started,
      game: liveGame,
      smartWaiting: scenario.waiting,
      paused: false,
      cpu: 1.4 + Math.random() * 0.6,
      memBytes: 148 * 1024 * 1024,
      saving: false,
      canSave: true,
    },
    usage: { usedBytes: clips.reduce((a, c) => a + c.size, 0), clipCount: clips.length, freeBytes: 412 * 1024 ** 3, totalBytes: 931 * 1024 ** 3, budgetBytes: 0 },
    ffmpeg: { ffmpeg: "ffmpeg.exe", ffprobe: "ffprobe.exe", version: "8.0" },
    ffmpegReady: !scenario.noFFmpeg,
    download: { active: false, done: 0, total: 0, error: "" },
    hotkeyErrors: [],
    activeJobs: jobs.filter((j) => j.status === "running" || j.status === "queued").length,
  };
}

setInterval(() => emit("status", status()), 1000);

function wave(n: number, seed: number) {
  const out: number[] = [];
  let v = 0.3;
  for (let i = 0; i < n; i++) {
    const r = Math.sin(i * 0.37 + seed) * Math.sin(i * 0.071 + seed * 3) + Math.sin(i * 1.9 + seed) * 0.3;
    v = v * 0.6 + Math.abs(r) * 0.4;
    out.push(Math.min(1, v * (i % 97 < 8 ? 2.2 : 1)));
  }
  return out;
}

const handlers: Record<string, (...a: any[]) => unknown> = {
  GetBootstrap: (): Bootstrap => ({
    version: "0.1.0",
    dataDir: "C:\\Users\\you\\AppData\\Local\\Snag",
    settings,
    status: status(),
    media: { base: "", token: "" },
    monitors: [
      { index: 0, left: 0, top: 0, width: 2560, height: 1440, primary: true },
      { index: 1, left: 2560, top: 0, width: 1920, height: 1080, primary: false },
    ],
    gpus: [{ name: "NVIDIA GeForce RTX 4060", vendor: "nvidia", driver: "596.49" }],
    update: { available: true, checking: false, latest: "v0.2.0", ready: scenario.update, error: "", checkedAt: Date.now() - 7 * 60000 },
    whatsNew: scenario.whatsNew
      ? { version: "0.2.0", notes: "## New\n- Custom uploaders from ShareX .sxcu files\n- A first-run setup\n\n## Fixed\n- Clips no longer stop short in the editor" }
      : null,
    encoders: [
      { name: "h264_nvenc", label: "NVENC H.264", codec: "h264", vendor: "nvidia", hardware: true, available: !scenario.noGpu, error: scenario.noGpu ? nvErr : undefined },
      { name: "hevc_nvenc", label: "NVENC HEVC", codec: "hevc", vendor: "nvidia", hardware: true, available: !scenario.noGpu, error: scenario.noGpu ? nvErr : undefined },
      { name: "h264_amf", label: "AMF H.264", codec: "h264", vendor: "amd", hardware: true, available: false },
      { name: "libx264", label: "x264 (CPU)", codec: "h264", vendor: "cpu", hardware: false, available: true },
    ],
    features: { ddagrab: true, initialised: true },
    clips,
    folders: sortedFolders(),
    jobs,
  }),
  UpdateClips: (ids: string[], p: Partial<Clip>) => {
    clips.filter((c) => ids.includes(c.id)).forEach((c) => Object.assign(c, p));
    emit("clips:changed", null);
  },
  ListFolders: sortedFolders,
  CreateFolder: (name: string) => {
    const f = { id: Math.random().toString(36).slice(2), name: folderName(name), createdAt: new Date().toISOString() };
    folders.push(f);
    emit("folders:changed", sortedFolders());
    return f;
  },
  RenameFolder: (id: string, name: string) => {
    const f = folders.find((f) => f.id === id)!;
    f.name = folderName(name, id);
    emit("folders:changed", sortedFolders());
    return f;
  },
  DeleteFolder: (id: string) => {
    folders = folders.filter((f) => f.id !== id);
    clips.forEach((c) => c.folder === id && (c.folder = undefined));
    emit("folders:changed", sortedFolders());
    emit("clips:changed", null);
  },
  GetStatus: status,
  SaveSettings: (s: Settings) => (settings = s),
  EstimateBitrate: (s: Settings) => {
    const base = { low: 6000, medium: 10000, high: 16000, ultra: 24000 }[s.quality];
    const h = s.resolution === "native" ? 1440 : Number(s.resolution);
    const w = Math.round((h * 16) / 9);
    const kbps = Math.round((base * Math.pow((w * h) / 2073600, 0.75) * Math.pow(s.fps / 60, 0.6) * (s.codec === "hevc" ? 0.7 : 1)) / 100) * 100;
    const mb = ((kbps + 160) * 60) / 8 / 1024;
    return { width: w, height: h, kbps, mbPerMin: mb, bufferMB: (mb * s.bufferSeconds) / 60 };
  },
  ListAudioDevices: () => ({
    outputs: [
      { id: "a", name: "Speakers (Realtek(R) Audio)", isDefault: true },
      { id: "b", name: "Headphones (HyperX Cloud II)", isDefault: false },
    ],
    inputs: [{ id: "c", name: "Microphone (HyperX Cloud II)", isDefault: true }],
  }),
  ListClips: () => clips,
  UpdateClip: (id: string, p: Partial<Clip>) => {
    const c = clips.find((c) => c.id === id)!;
    Object.assign(c, p);
    emit("clips:changed", null);
    return c;
  },
  DeleteClips: (ids: string[]) => {
    ids.forEach((id) =>
      clips.splice(
        clips.findIndex((c) => c.id === id),
        1,
      ),
    );
    emit("clips:changed", null);
  },
  GetWaveform: (_id: string, track: number, n: number) => wave(n, track * 7 + 1),
  Export: (o: { clipId: string; preview: boolean }) => {
    const j: Job = {
      id: Math.random().toString(36).slice(2),
      clipId: o.clipId,
      label: o.preview ? "Preview" : "Fit 10 MB",
      kind: o.preview ? "preview" : "export",
      status: "running",
      progress: 0,
      error: "",
      output: "",
      size: 0,
      origSize: 0,
      replace: false,
      createdAt: Date.now(),
    };
    jobs.unshift(j);
    const t = setInterval(() => {
      j.progress = Math.min(1, j.progress + 0.08);
      if (j.progress >= 1) {
        j.status = "done";
        j.output = "C:\\Users\\you\\Videos\\Snag\\Exports\\clip.mp4";
        j.size = 9.4 * 1024 * 1024;
        clearInterval(t);
      }
      emit("jobs:changed", [...jobs]);
    }, 250);
    return j;
  },
  ListJobs: () => jobs,
  ClearJobs: () => {
    jobs.splice(0, jobs.length, ...jobs.filter((j) => j.status === "running"));
    emit("jobs:changed", [...jobs]);
  },
  SaveClip: () => setTimeout(() => emit("toast", { kind: "success", message: `Saved ${liveGame} 2026-10-24 19-41-03` }), 300),
  AddBookmark: () => emit("toast", { kind: "info", message: "Bookmark added" }),
  PickFolder: () => "C:\\Users\\you\\Videos\\Snag",
  CopyClip: () => undefined,
  AIProviders: () => [
    { id: "groq", label: "Groq", model: "qwen/qwen3.8-27b", transcribe: true, keyUrl: "https://console.groq.com/keys" },
    { id: "gemini", label: "Gemini", model: "gemini-3.5-flash-lite", transcribe: true, keyUrl: "https://aistudio.google.com/apikey" },
    { id: "openai", label: "OpenAI", model: "gpt-6-luna", transcribe: true, keyUrl: "https://platform.openai.com/api-keys" },
    { id: "anthropic", label: "Claude", model: "claude-haiku-5-5", transcribe: false, keyUrl: "https://console.anthropic.com/settings/keys" },
  ],
  SetAIKey: () => {
    settings = { ...settings, aiKey: "saved" };
  },
  ShrinkClips: (ids: string[]) => ids.length,
  DescribeClip: (id: string) => {
    const c = clips.find((c) => c.id === id)!;
    c.title = "triple kill on the ramp";
    c.aiTitled = true;
    emit("clips:changed", null);
    return c;
  },
  TranscribeClip: (id: string) => {
    const c = clips.find((c) => c.id === id)!;
    c.transcript = "no way he did not see me, ok ok push now, they are all one shot";
    return c;
  },
  MergeClips: () => ({
    id: "m1",
    clipId: clips[0].id,
    label: "Merge 2 clips",
    kind: "merge",
    status: "queued",
    progress: 0,
    error: "",
    output: "",
    size: 0,
    origSize: 0,
    replace: false,
    createdAt: Date.now(),
  }),
  TakeScreenshot: () => undefined,
  ListUploaders: () => [
    { id: "catbox", name: "Catbox", host: "catbox.moe", builtin: true, videoReady: true },
    { id: "zipline-1a2b3c", name: "My Zipline", host: "files.example.dev", builtin: false, videoReady: true },
  ],
  ImportUploader: () => null,
  GetEncoders: () => [{ name: "h264_nvenc", label: "NVENC H.264", codec: "h264", vendor: "nvidia", hardware: true, available: true }],
  Diagnostics: () => "Snag diagnostics",
  CheckUpdate: () => ({ available: true, checking: false, latest: "v0.1.0", ready: false, error: "", checkedAt: Date.now() }),
  CopyPath: () => undefined,
  PreviewFileName: (s: Settings) => {
    const d = new Date();
    const p = (n: number) => String(n).padStart(2, "0");
    const name = (s.fileTemplate || "{game} {date} {time}")
      .replaceAll("{game}", liveGame)
      .replaceAll("{date}", `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`)
      .replaceAll("{time}", `${p(d.getHours())}-${p(d.getMinutes())}-${p(d.getSeconds())}`)
      .replaceAll("{length}", `${s.bufferSeconds}s`)
      .replaceAll("{year}", String(d.getFullYear()))
      .replaceAll("{month}", p(d.getMonth() + 1))
      .replaceAll("{day}", p(d.getDate()))
      .replace(/[<>:"/\\|?*]/g, "");
    return (s.organizeByGame ? liveGame + "\\" : "") + name + ".mp4";
  },
};

export const mock = {
  call: async (method: string, args: unknown[]) => {
    const h = handlers[method];
    return h ? h(...args) : undefined;
  },
  on: (name: string, cb: Listener) => {
    if (!listeners.has(name)) listeners.set(name, new Set());
    listeners.get(name)!.add(cb);
    return () => listeners.get(name)!.delete(cb);
  },
};
