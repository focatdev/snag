import * as S from "@radix-ui/react-slider";
import { cn } from "@/lib/format";

export function Slider({ className, ...props }: S.SliderProps) {
  const count = (props.value ?? props.defaultValue ?? [0]).length;
  return (
    <S.Root className={cn("relative flex h-5 w-full touch-none select-none items-center", className)} {...props}>
      <S.Track className="relative h-[3px] grow overflow-hidden rounded-full bg-line">
        <S.Range className="absolute h-full bg-frost" />
      </S.Track>
      {Array.from({ length: count }).map((_, i) => (
        <S.Thumb
          key={i}
          className="block h-3.5 w-3.5 rounded-full border-2 border-canvas bg-frost shadow-[0_0_0_1px_#2a2d31] transition-transform hover:scale-110 focus-visible:outline-2 focus-visible:outline-frost"
        />
      ))}
    </S.Root>
  );
}
