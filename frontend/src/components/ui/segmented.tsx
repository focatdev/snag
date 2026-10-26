import { cn } from "@/lib/format";

export function Segmented<T extends string | number>({ value, onChange, options, className, size = "md" }: { value: T; onChange: (v: T) => void; options: { value: T; label: string }[]; className?: string; size?: "sm" | "md" }) {
  return (
    <div role="radiogroup" className={cn("inline-flex rounded-ctl border border-line bg-well p-0.5", className)}>
      {options.map((o) => {
        const on = o.value === value;
        return (
          <button
            key={String(o.value)}
            role="radio"
            aria-checked={on}
            onClick={() => onChange(o.value)}
            className={cn(
              "flex-1 whitespace-nowrap rounded-[4px] px-2.5 transition-colors duration-100",
              size === "sm" ? "h-6 text-xs" : "h-7 text-sm",
              on ? "bg-hover text-fg shadow-[inset_0_0_0_1px_#34383d]" : "text-muted hover:text-fg",
            )}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
