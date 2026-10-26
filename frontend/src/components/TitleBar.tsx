import { Minus, Square, X } from "@phosphor-icons/react";
import { win } from "@/lib/api";
import { useStore } from "@/lib/store";
import { Mark } from "./Logo";

export function TitleBar() {
  const live = useStore((s) => s.status?.recorder.state === "buffering");
  return (
    <header className="drag flex h-9 shrink-0 items-center border-b border-line-soft bg-canvas" onDoubleClick={() => win.toggleMaximise()}>
      <div className="flex items-center gap-2 pl-4 text-fg">
        <Mark size={19} live={live} />
        <span className="display text-[15px] tracking-[-0.01em]">snag</span>
      </div>
      <div className="flex-1" />
      <div className="no-drag flex h-full" onDoubleClick={(e) => e.stopPropagation()}>
        <button aria-label="Minimize" onClick={() => win.minimise()} className="grid h-full w-[46px] place-items-center text-muted hover:bg-hover hover:text-fg">
          <Minus size={14} />
        </button>
        <button aria-label="Maximize" onClick={() => win.toggleMaximise()} className="grid h-full w-[46px] place-items-center text-muted hover:bg-hover hover:text-fg">
          <Square size={12} />
        </button>
        <button aria-label="Close" onClick={() => win.close()} className="grid h-full w-[46px] place-items-center text-muted hover:bg-[#c42b1c] hover:text-white">
          <X size={14} />
        </button>
      </div>
    </header>
  );
}
