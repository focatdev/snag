import * as S from "@radix-ui/react-select";
import { CaretDown, Check } from "@phosphor-icons/react";
import { cn } from "@/lib/format";

export type Option = { value: string; label: string; hint?: string; disabled?: boolean };

export function Select({ value, onChange, options, className, placeholder }: { value: string; onChange: (v: string) => void; options: Option[]; className?: string; placeholder?: string }) {
  return (
    <S.Root value={value} onValueChange={onChange}>
      <S.Trigger
        className={cn(
          "inline-flex h-8 min-w-[150px] items-center justify-between gap-2 rounded-ctl border border-line bg-raised px-2.5 text-sm text-fg outline-none hover:border-[#363a3f] focus-visible:outline-2 focus-visible:outline-frost data-[placeholder]:text-muted",
          className,
        )}
      >
        <span className="truncate">
          <S.Value placeholder={placeholder} />
        </span>
        <S.Icon>
          <CaretDown size={12} className="text-muted" />
        </S.Icon>
      </S.Trigger>
      <S.Portal>
        <S.Content position="popper" sideOffset={4} className="anim-pop z-50 max-h-[320px] min-w-[var(--radix-select-trigger-width)] overflow-hidden rounded-[8px] border border-line bg-raised p-1 shadow-[0_12px_32px_rgba(0,0,0,0.45)]">
          <S.Viewport>
            {options.map((o) => (
              <S.Item
                key={o.value}
                value={o.value}
                disabled={o.disabled}
                className="relative flex cursor-default items-center gap-2 rounded-[4px] py-1.5 pl-7 pr-3 text-sm text-fg outline-none data-[disabled]:opacity-40 data-[highlighted]:bg-hover"
              >
                <S.ItemIndicator className="absolute left-2">
                  <Check size={12} weight="bold" />
                </S.ItemIndicator>
                <S.ItemText>{o.label}</S.ItemText>
                {o.hint && <span className="ml-auto pl-4 text-xs text-faint">{o.hint}</span>}
              </S.Item>
            ))}
          </S.Viewport>
        </S.Content>
      </S.Portal>
    </S.Root>
  );
}
