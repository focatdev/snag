import { CheckCircle, Info, WarningCircle } from "@phosphor-icons/react";
import { useStore } from "@/lib/store";

export function Toasts() {
  const toasts = useStore((s) => s.toasts);
  const dismiss = useStore((s) => s.dismiss);
  return (
    <div className="pointer-events-none fixed bottom-10 right-4 z-[60] flex w-[340px] flex-col items-end gap-2" role="status" aria-live="polite">
      {toasts.map((t) => (
        <button
          key={t.id}
          onClick={() => dismiss(t.id)}
          className="anim-toast pointer-events-auto flex max-w-full items-start gap-2.5 rounded-[8px] border border-line bg-[#1f2124] px-3 py-2.5 text-left text-sm text-fg shadow-[0_10px_30px_rgba(0,0,0,0.45)]"
        >
          {t.kind === "success" && <CheckCircle size={16} weight="fill" className="mt-px shrink-0 text-ok" />}
          {t.kind === "error" && <WarningCircle size={16} weight="fill" className="mt-px shrink-0 text-bad" />}
          {t.kind === "info" && <Info size={16} weight="fill" className="mt-px shrink-0 text-muted" />}
          <span className="min-w-0 break-words">{t.message}</span>
        </button>
      ))}
    </div>
  );
}
