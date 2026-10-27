import * as M from "@radix-ui/react-dropdown-menu";
import * as C from "@radix-ui/react-context-menu";
import { CaretRight } from "@phosphor-icons/react";
import type { ElementType, ReactNode } from "react";
import { cn } from "@/lib/format";

const content = "anim-pop z-50 min-w-[180px] max-w-[280px] rounded-[8px] border border-line bg-raised p-1 shadow-[0_12px_32px_rgba(0,0,0,0.45)]";
const item = "flex cursor-default items-center gap-2 rounded-[4px] px-2 py-1.5 text-sm text-fg outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-40 [&_svg]:text-muted";

export type MenuEntry =
  | { label: string; icon?: ReactNode; onSelect: () => void; danger?: boolean; disabled?: boolean }
  | { label: string; icon?: ReactNode; sub: MenuEntry[]; disabled?: boolean }
  | "sep";

// a dialog opened from a menu item keeps focus instead of the menu trigger
const keepDialogFocus = (e: Event) => document.querySelector("[role=dialog]") && e.preventDefault();

// dropdown and context menus share item markup, only the radix parts differ
type Parts = Record<"Item" | "Separator" | "Sub" | "SubTrigger" | "SubContent" | "Portal", ElementType>;

function Entries({ P, items }: { P: Parts; items: MenuEntry[] }) {
  return items.map((it, i) =>
    it === "sep" ? (
      <P.Separator key={i} className="my-1 h-px bg-line" />
    ) : "sub" in it ? (
      <P.Sub key={i}>
        <P.SubTrigger disabled={it.disabled} className={cn(item, "data-[state=open]:bg-hover")}>
          {it.icon}
          <span className="flex-1">{it.label}</span>
          <CaretRight size={12} />
        </P.SubTrigger>
        <P.Portal>
          <P.SubContent sideOffset={6} alignOffset={-5} className={cn(content, "max-h-[320px] overflow-y-auto")}>
            <Entries P={P} items={it.sub} />
          </P.SubContent>
        </P.Portal>
      </P.Sub>
    ) : (
      <P.Item key={i} disabled={it.disabled} onSelect={it.onSelect} className={cn(item, it.danger && "text-bad [&_svg]:text-bad")}>
        {it.icon}
        <span className="truncate">{it.label}</span>
      </P.Item>
    ),
  );
}

export function DropMenu({ trigger, items, align = "end" }: { trigger: ReactNode; items: MenuEntry[]; align?: "start" | "end" }) {
  return (
    <M.Root>
      <M.Trigger asChild>{trigger}</M.Trigger>
      <M.Portal>
        <M.Content align={align} sideOffset={4} className={content} onCloseAutoFocus={keepDialogFocus}>
          <Entries P={M as unknown as Parts} items={items} />
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
        <C.Content className={content} onCloseAutoFocus={keepDialogFocus}>
          <Entries P={C as unknown as Parts} items={items} />
        </C.Content>
      </C.Portal>
    </C.Root>
  );
}
