import * as S from "@radix-ui/react-switch";
import { cn } from "@/lib/format";

export function Switch({ className, ...props }: S.SwitchProps) {
  return (
    <S.Root
      className={cn(
        "relative inline-flex h-[18px] w-8 shrink-0 cursor-pointer items-center rounded-full border border-line bg-well transition-colors data-[state=checked]:border-frost data-[state=checked]:bg-frost disabled:opacity-40",
        className,
      )}
      {...props}
    >
      <S.Thumb className="block h-3 w-3 translate-x-[2px] rounded-full bg-muted transition-transform duration-100 data-[state=checked]:translate-x-[15px] data-[state=checked]:bg-frost-ink" />
    </S.Root>
  );
}
