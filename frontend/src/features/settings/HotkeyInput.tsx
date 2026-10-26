import { useEffect, useState } from "react";
import { Kbd } from "@/components/ui/kbd";
import { cn } from "@/lib/format";

const named: Record<string, string> = {
  Space: "Space",
  Tab: "Tab",
  Enter: "Enter",
  Backspace: "Backspace",
  Delete: "Delete",
  Home: "Home",
  End: "End",
  PageUp: "Page Up",
  PageDown: "Page Down",
  ArrowUp: "Up",
  ArrowDown: "Down",
  ArrowLeft: "Left",
  ArrowRight: "Right",
};

// wails wants key names, not codes
function toAccelerator(e: KeyboardEvent): string | null {
  let key = "";
  if (/^F([1-9]|1[0-9]|2[0-4])$/.test(e.code)) key = e.code;
  else if (/^Key[A-Z]$/.test(e.code)) key = e.code.slice(3);
  else if (/^Digit[0-9]$/.test(e.code)) key = e.code.slice(5);
  else if (named[e.code]) key = named[e.code];
  if (!key) return null;
  const mods = [e.ctrlKey && "Ctrl", e.altKey && "Alt", e.shiftKey && "Shift", e.metaKey && "Super"].filter(Boolean) as string[];
  const isF = key.startsWith("F") && key.length > 1;
  // bare letters would fire while typing
  if (!isF && mods.length === 0) return null;
  return [...mods, key].join("+");
}

export function HotkeyInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const [listening, setListening] = useState(false);
  const [hint, setHint] = useState("");

  useEffect(() => {
    if (!listening) return;
    const onKey = (e: KeyboardEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (e.key === "Escape") {
        setListening(false);
        return;
      }
      if ((e.key === "Backspace" || e.key === "Delete") && !e.ctrlKey && !e.altKey && !e.shiftKey) {
        onChange("");
        setListening(false);
        return;
      }
      if (["Control", "Alt", "Shift", "Meta"].includes(e.key)) return;
      const acc = toAccelerator(e);
      if (!acc) {
        setHint("Add Ctrl, Alt or Shift, or use an F key");
        return;
      }
      onChange(acc);
      setListening(false);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [listening, onChange]);

  return (
    <button
      onClick={() => {
        setHint("");
        setListening(!listening);
      }}
      onBlur={() => setListening(false)}
      className={cn(
        "flex h-8 min-w-[180px] items-center justify-between gap-3 rounded-ctl border px-2.5 text-sm",
        listening ? "border-frost bg-well text-fg" : "border-line bg-raised hover:border-[#363a3f]",
      )}
    >
      {listening ? <span className="text-muted">{hint || "Press a key combo"}</span> : <Kbd keys={value} />}
      <span className="text-2xs text-faint">{listening ? "Esc to cancel" : "Change"}</span>
    </button>
  );
}
