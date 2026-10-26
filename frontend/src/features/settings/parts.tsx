import type { ReactNode } from "react";
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
