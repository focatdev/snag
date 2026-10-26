import { forwardRef, type ButtonHTMLAttributes } from "react";
import { cn } from "@/lib/format";

type Variant = "primary" | "default" | "ghost" | "danger" | "ink";
type Size = "sm" | "md" | "icon" | "icon-sm";

const variants: Record<Variant, string> = {
  primary: "bg-frost text-frost-ink hover:bg-frost-hi active:bg-[#cdd6e2] font-medium",
  default: "bg-raised text-fg border border-line hover:bg-hover hover:border-[#363a3f]",
  ghost: "text-muted hover:text-fg hover:bg-hover",
  danger: "bg-raised text-bad border border-line hover:bg-[#2a1e1e] hover:border-[#4a2c2a]",
  ink: "bg-frost-ink text-frost-hi hover:bg-[#232a34] font-medium",
};

const sizes: Record<Size, string> = {
  sm: "h-7 px-2.5 text-xs gap-1.5",
  md: "h-8 px-3 text-sm gap-2",
  icon: "h-8 w-8 justify-center",
  "icon-sm": "h-7 w-7 justify-center",
};

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: Size };

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(({ className, variant = "default", size = "md", ...props }, ref) => (
  <button
    ref={ref}
    className={cn(
      "inline-flex shrink-0 items-center whitespace-nowrap rounded-ctl transition-colors duration-100 disabled:pointer-events-none disabled:opacity-40 [&_svg]:shrink-0",
      variants[variant],
      sizes[size],
      className,
    )}
    {...props}
  />
));
Button.displayName = "Button";
