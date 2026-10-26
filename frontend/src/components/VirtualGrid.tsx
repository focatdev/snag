import { useLayoutEffect, useRef, useState, type ReactNode } from "react";

export function VirtualGrid<T>({ items, minWidth, gapX, gapY, info, render, keyOf, className, children }: {
  items: T[];
  minWidth: number;
  gapX: number;
  gapY: number;
  info: number;
  render: (item: T) => ReactNode;
  keyOf: (item: T) => string;
  className?: string;
  children?: ReactNode;
}) {
  const scroller = useRef<HTMLDivElement>(null);
  const [box, setBox] = useState({ w: 0, h: 0 });
  const [top, setTop] = useState(0);

  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setBox({ w: e.contentRect.width, h: e.contentRect.height }));
    ro.observe(el);
    let raf = 0;
    const onScroll = () => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => setTop(el.scrollTop));
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      ro.disconnect();
      el.removeEventListener("scroll", onScroll);
      cancelAnimationFrame(raf);
    };
  }, []);

  const cols = Math.max(1, Math.floor((box.w + gapX) / (minWidth + gapX)));
  const cardW = box.w > 0 ? (box.w - gapX * (cols - 1)) / cols : minWidth;
  const rowH = Math.ceil((cardW * 9) / 16 + info + gapY);
  const rows = Math.ceil(items.length / cols);
  const first = Math.max(0, Math.floor(top / rowH) - 2);
  const last = Math.min(rows, Math.ceil((top + box.h) / rowH) + 2);

  const visible: ReactNode[] = [];
  for (let r = first; r < last; r++) {
    for (let c = 0; c < cols; c++) {
      const item = items[r * cols + c];
      if (item === undefined) break;
      visible.push(
        <div key={keyOf(item)} className="absolute" style={{ top: r * rowH, left: c * (cardW + gapX), width: cardW }}>
          {render(item)}
        </div>,
      );
    }
  }

  return (
    <div ref={scroller} className={className}>
      {children ?? (box.w > 0 && <div className="relative" style={{ height: rows * rowH - gapY }}>{visible}</div>)}
    </div>
  );
}
