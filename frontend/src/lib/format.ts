import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export const cn = (...v: ClassValue[]) => twMerge(clsx(v));

export function bytes(n: number, digits = 1) {
  if (!n || n < 0) return "0 MB";
  const gb = n / 1024 ** 3;
  if (gb >= 1) return `${gb >= 100 ? Math.round(gb) : gb.toFixed(digits)} GB`;
  const mb = n / 1024 ** 2;
  return `${mb >= 100 ? Math.round(mb) : mb.toFixed(digits)} MB`;
}

export function clock(sec: number, withTenths = false) {
  if (!isFinite(sec) || sec < 0) sec = 0;
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  const base = `${m}:${String(s).padStart(2, "0")}`;
  if (!withTenths) return base;
  return `${base}.${Math.floor((sec % 1) * 10)}`;
}

export function timecode(sec: number, fps = 60) {
  if (!isFinite(sec) || sec < 0) sec = 0;
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  const f = Math.floor((sec % 1) * fps);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}:${String(f).padStart(2, "0")}`;
}

export function relDate(iso: string) {
  const d = new Date(iso);
  const diff = (Date.now() - d.getTime()) / 1000;
  if (diff < 60) return "Just now";
  if (diff < 3600) return `${Math.floor(diff / 60)} min ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} h ago`;
  if (diff < 86400 * 2) return "Yesterday";
  if (diff < 86400 * 7) return d.toLocaleDateString(undefined, { weekday: "long" });
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: d.getFullYear() === new Date().getFullYear() ? undefined : "numeric" });
}

export function fullDate(iso: string) {
  return new Date(iso).toLocaleString(undefined, { month: "short", day: "numeric", year: "numeric", hour: "numeric", minute: "2-digit" });
}

export const resLabel = (h: number) => (h >= 2160 ? "4K" : h >= 1440 ? "1440p" : h >= 1080 ? "1080p" : h >= 720 ? "720p" : `${h}p`);

export function hotkeyParts(accel: string) {
  return accel ? accel.split("+") : [];
}
