/** SideDrawer is a page's options drawer: in the free space right of the page's card, or a sheet over the page on a phone, the page behind held still and a tap on the dim closing it; Escape closes it too. */
import { useEffect, useRef, type ReactNode } from 'react';

/** narrow is a phone's width, where the drawer is a sheet. */
export const narrow = () => window.innerWidth <= 768;

export default function SideDrawer({ className, onClose, children }: { className?: string; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    const place = () => {
      const side = ref.current;
      const layout = side?.parentElement;
      if (!side || !layout) return;
      if (narrow()) {
        side.style.left = '';
        side.style.right = '';
        return;
      }
      const card = layout.getBoundingClientRect();
      const width = document.documentElement.clientWidth;
      const own = side.offsetWidth || 360;
      const ideal = card.right + (width - card.right) / 2 - own / 2;
      const left = Math.max(16, Math.min(ideal, width - own - 16));
      // A fixed drawer is placed on the window, one in the page's flow on its layout.
      side.style.left = `${getComputedStyle(side).position === 'fixed' ? left : left - card.left}px`;
      side.style.right = 'auto';
    };
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    place();
    document.body.style.overflow = narrow() ? 'hidden' : '';
    window.addEventListener('resize', place);
    document.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = '';
      window.removeEventListener('resize', place);
      document.removeEventListener('keydown', onKey);
    };
  }, [onClose]);
  return (
    <aside ref={ref} className={`leaderboard-sidebar${className === undefined ? '' : ` ${className}`}`}>
      <button type="button" className="sidebar-backdrop" aria-label="Close options" tabIndex={-1} onClick={onClose} />
      {children}
    </aside>
  );
}

/** OptionsToggle opens and closes a page's drawer, marked Active while any option differs from its default. */
export function OptionsToggle({ title, active, open, onClick }: { title: string; active: boolean; open: boolean; onClick: () => void }) {
  return (
    <button type="button" className={['btn-options-toggle', active && 'active', open && 'open'].filter(Boolean).join(' ')} title={title} aria-expanded={open} onClick={onClick}>
      <span>⚙️ Options</span>
      {active && <span className="options-active-badge">Active</span>}
      <span className="btn-options-toggle-arrow">{open ? '✕' : '▶'}</span>
    </button>
  );
}
