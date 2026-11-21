/** Every table's box: it scrolls sideways when the table is wider, shades each side with more beyond it, says so on a phone, and can be focused and scrolled from the keyboard. */
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { scrollEdges, type Edges } from '../lib/scroll-table';

/** ScrollTable wraps one table; label names the box for screen readers, such as "Rivals". */
export default function ScrollTable({ label, children }: { label: string; children: ReactNode }) {
  const box = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState<Edges>({ start: false, end: false });

  useEffect(() => {
    const el = box.current;
    if (el === null) return;
    const update = () => setEdges(scrollEdges(el.scrollLeft, el.clientWidth, el.scrollWidth));
    update();
    el.addEventListener('scroll', update, { passive: true });
    const resized = new ResizeObserver(update);
    resized.observe(el);
    if (el.firstElementChild !== null) resized.observe(el.firstElementChild);
    return () => {
      el.removeEventListener('scroll', update);
      resized.disconnect();
    };
  }, []);

  const scrolls = edges.start || edges.end;
  return (
    <div className="scroll-table">
      {scrolls && <p className="scroll-hint">Swipe table horizontally for more ➔</p>}
      <div className={`scroll-table-box${edges.start ? ' more-start' : ''}${edges.end ? ' more-end' : ''}`}>
        {/* eslint-disable-next-line jsx-a11y-x/no-noninteractive-tabindex -- a box that scrolls must take keyboard focus (WCAG 2.1.1), as the rule's docs advise */}
        <div ref={box} className={edges.start ? 'table-responsive scrolled-x' : 'table-responsive'} tabIndex={0} role="region" aria-label={label}>
          {children}
        </div>
      </div>
    </div>
  );
}
