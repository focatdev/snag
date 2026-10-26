import * as T from "@radix-ui/react-tooltip";
import type { ReactNode } from "react";

export const TooltipProvider = T.Provider;

export function Tip({ label, children, side = "top" }: { label: ReactNode; children: ReactNode; side?: "top" | "bottom" | "left" | "right" }) {
  return (
    <T.Root delayDuration={350}>
      <T.Trigger asChild>{children}</T.Trigger>
      <T.Portal>
        <T.Content side={side} sideOffset={6} className="anim-fade z-50 rounded-[5px] border border-line bg-[#202226] px-2 py-1 text-xs text-fg shadow-[0_6px_20px_rgba(0,0,0,0.4)]">
          {label}
        </T.Content>
      </T.Portal>
    </T.Root>
  );
}
