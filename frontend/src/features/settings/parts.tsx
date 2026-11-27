import { useEffect, useState, type InputHTMLAttributes, type ReactNode } from "react";
import { Input } from "@/components/ui/input";
import type { Settings as S } from "@/lib/api";

export { Row } from "@/components/ui/row";

export type SectionProps = { s: S; set: (p: Partial<S>) => void };

export function Section({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section id={`s-${id}`} className="scroll-mt-6 pb-8">
      <h2 className="mb-1 text-lg font-semibold">{title}</h2>
      <div>{children}</div>
    </section>
  );
}

export function DraftInput({
  value,
  onCommit,
  allowEmpty = false,
  ...rest
}: {
  value: string;
  onCommit: (v: string) => void;
  allowEmpty?: boolean;
} & Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange">) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [value]);
  const commit = () => {
    const v = draft.trim();
    if (!v && !allowEmpty) return setDraft(value);
    if (v !== value) onCommit(v);
  };
  return (
    <Input
      spellCheck={false}
      {...rest}
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => e.key === "Enter" && commit()}
    />
  );
}
