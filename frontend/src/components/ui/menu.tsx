import * as M from "@radix-ui/react-dropdown-menu";
import * as C from "@radix-ui/react-context-menu";
import type { ReactNode } from "react";
import { cn } from "@/lib/format";

const content = "anim-pop z-50 min-w-[180px] rounded-[8px] border border-line bg-raised p-1 shadow-[0_12px_32px_rgba(0,0,0,0.45)]";
const item = "flex cursor-default items-center gap-2 rounded-[4px] px-2 py-1.5 text-sm text-fg outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-40 [&_svg]:text-muted";

export type MenuEntry = { label: string; icon?: ReactNode; onSelect: () => void; danger?: boolean; disabled?: boolean } | "sep";

export function DropMenu({ trigger, items, align = "end" }: { trigger: ReactNode; items: MenuEntry[]; align?: "start" | "end" }) {
  return (
    <M.Root>
      <M.Trigger asChild>{trigger}</M.Trigger>
      <M.Portal>
        <M.Content align={align} sideOffset={4} className={content}>
          {items.map((it, i) =>
            it === "sep" ? (
              <M.Separator key={i} className="my-1 h-px bg-line" />
            ) : (
              <M.Item key={i} disabled={it.disabled} onSelect={it.onSelect} className={cn(item, it.danger && "text-bad [&_svg]:text-bad")}>
                {it.icon}
                {it.label}
              </M.Item>
            ),
          )}
        </M.Content>
      </M.Portal>
    </M.Root>
  );
}

export function CtxMenu({ children, items }: { children: ReactNode; items: MenuEntry[] }) {
  return (
    <C.Root>
      <C.Trigger asChild>{children}</C.Trigger>
      <C.Portal>
        <C.Content className={content}>
          {items.map((it, i) =>
            it === "sep" ? (
              <C.Separator key={i} className="my-1 h-px bg-line" />
            ) : (
              <C.Item key={i} disabled={it.disabled} onSelect={it.onSelect} className={cn(item, it.danger && "text-bad [&_svg]:text-bad")}>
                {it.icon}
                {it.label}
              </C.Item>
            ),
          )}
        </C.Content>
      </C.Portal>
    </C.Root>
  );
}
