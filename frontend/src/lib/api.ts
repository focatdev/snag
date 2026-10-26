import { Browser, Call, Events, Window as WailsWindow } from "@wailsio/runtime";
import { mock } from "./mock";

export type Hotkeys = { saveClip: string; saveShort: string; screenshot: string; bookmark: string; toggleBuffer: string };

export type Settings = {
  clipsDir: string;
  exportsDir: string;
  organizeByGame: boolean;
  fileTemplate: string;
  ffmpegPath: string;
  ffmpegDir: string;
  encoder: string;
  codec: "h264" | "hevc";
  fps: number;
  resolution: "native" | "1440" | "1080" | "720";
  quality: "low" | "medium" | "high" | "ultra";
  monitor: number;
  drawCursor: boolean;
  bufferSeconds: number;
  shortSeconds: number;
  captureSystemAudio: boolean;
  captureMic: boolean;
  separateVoice: boolean;
  dropSilentMic: boolean;
  bufferOnDisk: boolean;
  mergeOverlaps: boolean;
  autoShrinkDays: number;
  copyScreenshots: boolean;
  catboxAutoUpload: boolean;
  catboxDeleteAfter: boolean;
  catboxUserhash: string;
  uploader: string;
  aiProvider: "" | "groq" | "gemini" | "openai" | "anthropic";
  aiKey: string;
  aiModel: string;
  aiAutoTitle: boolean;
  aiTranscribe: boolean;
  systemDeviceId: string;
  micDeviceId: string;
  hotkeys: Hotkeys;
  smartMode: boolean;
  autoStartBuffer: boolean;
  saveSound: boolean;
  saveSoundName: "snap" | "chime" | "pop" | "shutter" | "custom";
  soundVolume: number;
  customSound: string;
  showOverlay: boolean;
  overlayCorner: "top-left" | "top-right" | "bottom-left" | "bottom-right";
  copyExports: boolean;
  storageBudgetEnabled: boolean;
  storageBudgetGB: number;
  launchOnStartup: boolean;
  startMinimized: boolean;
  closeToTray: boolean;
  autoUpdate: boolean;
  onboarded: boolean;
  lastVersion: string;
};

export type RecorderState = "idle" | "starting" | "buffering" | "error";

export type RecorderStatus = {
  state: RecorderState;
  error: string;
  warning: string;
  encoder: string;
  encoderLabel: string;
  pipeline: string;
  width: number;
  height: number;
  fps: number;
  kbps: number;
  bufferedSec: number;
  bufferTarget: number;
  bufferBytes: number;
  tracks: string[] | null;
  startedAt: number;
  game: string;
  smartWaiting: boolean;
  paused: boolean;
  cpu: number;
  memBytes: number;
  saving: boolean;
  canSave: boolean;
};

export type Usage = { usedBytes: number; clipCount: number; freeBytes: number; totalBytes: number; budgetBytes: number };

export type AppStatus = {
  recorder: RecorderStatus;
  usage: Usage;
  ffmpeg: { ffmpeg: string; ffprobe: string; version: string };
  ffmpegReady: boolean;
  download: { active: boolean; done: number; total: number; error: string };
  hotkeyErrors: string[];
  activeJobs: number;
};

export type Clip = {
  id: string;
  path: string;
  title: string;
  game: string;
  createdAt: string;
  duration: number;
  width: number;
  height: number;
  fps: number;
  size: number;
  codec: string;
  tracks: string[] | null;
  markers: number[] | null;
  favorite: boolean;
  tags: string[] | null;
  thumb: boolean;
  sprite: boolean;
  spriteFrames: number;
  export: boolean;
  probed: boolean;
  shrunk: boolean;
  origSize: number;
  aiTitled: boolean;
  transcript: string;
  link?: string;
  deleteLink?: string;
};

export type Encoder = { name: string; label: string; codec: string; vendor: string; hardware: boolean; available: boolean; error?: string; detail?: string };
export type GPU = { name: string; vendor: string; driver: string };
export type Monitor = { index: number; left: number; top: number; width: number; height: number; primary: boolean };
export type AudioDevice = { id: string; name: string; isDefault: boolean };

export type TrackMix = { index: number; volume: number; muted: boolean };

export type ExportOptions = {
  clipId: string;
  start: number;
  end: number;
  fast: boolean;
  format: "mp4" | "gif";
  targetMB: number;
  height: number;
  fps: number;
  speed: number;
  motionBlur: "off" | "subtle" | "medium" | "heavy";
  vertical: boolean;
  cropX: number;
  tracks: TrackMix[];
  mixAudio: boolean;
  name: string;
  preview: boolean;
  cuts: [number, number][];
  replaceOriginal: boolean;
};

export type UpdateState = { available: boolean; checking: boolean; latest: string; ready: boolean; error: string; checkedAt: number };

export type Uploader = { id: string; name: string; host: string; builtin: boolean; videoReady: boolean };

export type AIProvider = { id: string; label: string; model: string; transcribe: boolean; keyUrl: string };

export type Job = {
  id: string;
  clipId: string;
  label: string;
  kind: "export" | "preview" | "shrink" | "merge" | "upload";
  status: "queued" | "running" | "done" | "failed" | "canceled";
  progress: number;
  error: string;
  output: string;
  size: number;
  origSize: number;
  replace: boolean;
  createdAt: number;
};

export type Bootstrap = {
  version: string;
  dataDir: string;
  settings: Settings;
  status: AppStatus;
  media: { base: string; token: string };
  monitors: Monitor[];
  encoders: Encoder[] | null;
  gpus: GPU[] | null;
  update: UpdateState;
  whatsNew: { version: string; notes: string } | null;
  features: { ddagrab: boolean; initialised: boolean };
  clips: Clip[] | null;
  jobs: Job[] | null;
};

export type Estimate = { width: number; height: number; kbps: number; mbPerMin: number; bufferMB: number };

let useMock = false;

const call = <T,>(method: string, ...args: unknown[]): Promise<T> => {
  if (useMock) return mock.call(method, args) as Promise<T>;
  return Call.ByName(`main.SnagService.${method}`, ...args) as Promise<T>;
};

export async function connect(): Promise<Bootstrap> {
  const wantMock = new URLSearchParams(location.search).has("mock");
  if (!wantMock) {
    try {
      return await call<Bootstrap>("GetBootstrap");
    } catch (e) {
      if (!import.meta.env.DEV) throw e;
    }
  }
  useMock = true;
  return mock.call("GetBootstrap", []) as Promise<Bootstrap>;
}

export const api = {
  status: () => call<AppStatus>("GetStatus"),
  saveSettings: (s: Settings) => call<Settings>("SaveSettings", s),
  estimate: (s: Settings) => call<Estimate>("EstimateBitrate", s),
  startBuffer: () => call<void>("StartBuffer"),
  stopBuffer: () => call<void>("StopBuffer"),
  saveClip: () => call<void>("SaveClip"),
  saveShortClip: () => call<void>("SaveShortClip"),
  copyClip: (id: string) => call<void>("CopyClip", id),
  copyPath: (p: string) => call<void>("CopyPath", p),
  previewFileName: (s: Settings) => call<string>("PreviewFileName", s),
  testOverlay: () => call<void>("TestOverlay"),
  shrinkClips: (ids: string[]) => call<number>("ShrinkClips", ids),
  mergeClips: (ids: string[], name: string) => call<Job>("MergeClips", ids, name),
  takeScreenshot: () => call<void>("TakeScreenshot"),
  repairClip: (id: string) => call<Clip>("RepairClip", id),
  checkUpdate: () => call<UpdateState>("CheckUpdate"),
  restartToUpdate: () => call<void>("RestartToUpdate"),
  diagnostics: () => call<string>("Diagnostics"),
  openLogFolder: () => call<void>("OpenLogFolder"),
  encoders: () => call<Encoder[]>("GetEncoders"),
  listUploaders: () => call<Uploader[]>("ListUploaders"),
  importUploader: () => call<Uploader | null>("ImportUploader"),
  importUploaderText: (text: string) => call<Uploader>("ImportUploaderText", text),
  deleteUploader: (id: string) => call<void>("DeleteUploader", id),
  uploadClip: (id: string) => call<Job>("UploadClip", id),
  copyText: (t: string) => call<void>("CopyText", t),
  describeClip: (id: string) => call<Clip>("DescribeClip", id),
  transcribeClip: (id: string) => call<Clip>("TranscribeClip", id),
  aiProviders: () => call<AIProvider[]>("AIProviders"),
  setAIKey: (key: string) => call<void>("SetAIKey", key),
  previewSound: (s: Settings) => call<void>("PreviewSound", s),
  pickSoundFile: () => call<string>("PickSoundFile"),
  addBookmark: () => call<void>("AddBookmark"),
  downloadFFmpeg: () => call<void>("DownloadFFmpeg"),
  reprobe: () => call<Encoder[]>("ReprobeEncoders"),
  audioDevices: () => call<{ outputs: AudioDevice[]; inputs: AudioDevice[] }>("ListAudioDevices"),
  monitors: () => call<Monitor[]>("ListMonitors"),
  clips: () => call<Clip[]>("ListClips"),
  updateClip: (id: string, patch: Partial<Pick<Clip, "title" | "favorite" | "tags" | "game">>) => call<Clip>("UpdateClip", id, patch),
  deleteClips: (ids: string[]) => call<void>("DeleteClips", ids),
  rescan: () => call<void>("Rescan"),
  waveform: (id: string, track: number, buckets: number) => call<number[]>("GetWaveform", id, track, buckets),
  revealClip: (id: string) => call<void>("RevealClip", id),
  revealPath: (p: string) => call<void>("RevealPath", p),
  openClipsFolder: () => call<void>("OpenClipsFolder"),
  exportClip: (o: ExportOptions) => call<Job>("Export", o),
  jobs: () => call<Job[]>("ListJobs"),
  cancelJob: (id: string) => call<void>("CancelJob", id),
  clearJobs: () => call<void>("ClearJobs"),
  pickFolder: (start: string) => call<string>("PickFolder", start),
  pickFFmpeg: () => call<string>("PickFFmpeg"),
  quit: () => call<void>("Quit"),
};

export function on<T>(name: string, cb: (data: T) => void): () => void {
  if (useMock) return mock.on(name, cb as (d: unknown) => void);
  return Events.On(name, (ev: { data: unknown }) => cb(ev.data as T));
}

export const openUrl = (url: string) => (useMock ? window.open(url, "_blank") : Browser.OpenURL(url));

export const win = {
  minimise: () => (useMock ? undefined : WailsWindow.Minimise()),
  toggleMaximise: () => (useMock ? undefined : WailsWindow.ToggleMaximise()),
  close: () => (useMock ? undefined : WailsWindow.Close()),
};
