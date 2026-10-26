import { cn } from "@/lib/format";

export function Kbd({ keys, className, tone = "dark" }: { keys: string; className?: string; tone?: "dark" | "frost" }) {
  if (!keys) return <span className="text-faint">Not set</span>;
  return (
    <span className={cn("inline-flex items-center gap-1", className)}>
      {keys.split("+").map((k, i) => (
        <kbd
          key={i}
          className={cn(
            "inline-flex h-5 min-w-5 items-center justify-center rounded-[4px] px-1.5 font-sans text-2xs font-medium",
            tone === "dark" ? "border border-line bg-well text-fg shadow-[0_1px_0_#2a2d31]" : "border border-[#b9c3d1] bg-frost-hi/70 text-frost-ink shadow-[0_1px_0_#b9c3d1]",
          )}
        >
          {k}
        </kbd>
      ))}
    </span>
  );
}
