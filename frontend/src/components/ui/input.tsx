import { forwardRef, type InputHTMLAttributes } from "react";
import { cn } from "@/lib/format";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(({ className, ...props }, ref) => (
  <input
    ref={ref}
    className={cn(
      "h-8 rounded-ctl border border-line bg-well px-2.5 text-sm text-fg outline-none placeholder:text-faint hover:border-[#363a3f] focus:border-[#4a5260]",
      className,
    )}
    {...props}
  />
));
Input.displayName = "Input";
