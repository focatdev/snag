import type { ReactNode } from "react";
import { cn } from "@/lib/format";

export function Row({ title, desc, children, wide, dense }: { title: string; desc?: ReactNode; children: ReactNode; wide?: boolean; dense?: boolean }) {
  return (
    <div className={cn("flex border-b border-line-soft last:border-b-0", dense ? "gap-6 py-3.5" : "gap-8 py-4", wide ? "flex-col gap-3" : "items-center justify-between")}>
      <div className="max-w-[420px]">
        <p className="text-sm text-fg">{title}</p>
        {desc && <p className="mt-0.5 text-xs leading-relaxed text-muted">{desc}</p>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}
