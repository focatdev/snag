import * as D from "@radix-ui/react-dialog";
import { X } from "@phosphor-icons/react";
import type { ReactNode } from "react";
import { cn } from "@/lib/format";

export function Dialog({ open, onOpenChange, title, description, children, className }: { open: boolean; onOpenChange: (o: boolean) => void; title: string; description?: string; children: ReactNode; className?: string }) {
  return (
    <D.Root open={open} onOpenChange={onOpenChange}>
      <D.Portal>
        <D.Overlay className="anim-fade fixed inset-0 z-40 bg-black/55" />
        <D.Content
          className={cn(
            "anim-pop fixed left-1/2 top-1/2 z-50 w-[420px] max-w-[calc(100vw-32px)] -translate-x-1/2 -translate-y-1/2 rounded-card border border-line bg-raised p-5 shadow-[0_24px_64px_rgba(0,0,0,0.55)]",
            className,
          )}
        >
          <div className="mb-3 flex items-start justify-between gap-4">
            <div>
              <D.Title className="text-base font-semibold">{title}</D.Title>
              {description && <D.Description className="mt-1 text-sm text-muted">{description}</D.Description>}
            </div>
            <D.Close className="-mr-1 -mt-1 rounded-ctl p-1 text-muted hover:bg-hover hover:text-fg" aria-label="Close">
              <X size={14} />
            </D.Close>
          </div>
          {children}
        </D.Content>
      </D.Portal>
    </D.Root>
  );
}
