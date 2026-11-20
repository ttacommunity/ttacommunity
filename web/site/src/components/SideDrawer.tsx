/** SideDrawer is a page's options drawer: in the free space right of the page's card, or a sheet over the page on a phone, the page behind held still and a tap on the dim closing it; Escape closes it too. */
import { useEffect, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import type { Link } from '../lib/navigate';
import './SideDrawer.css';

/** narrow is a phone's width, where the drawer is a sheet. */
export const narrow = () => window.innerWidth <= 768;

export default function SideDrawer({ onClose, children }: { onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLElement>(null);
  const anchor = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    const place = () => {
      const side = ref.current;
      let layout = anchor.current?.parentElement ?? null;
      // An island's wrapper has no box of its own: place beside the first ancestor that has one.
      while (layout !== null && layout.getBoundingClientRect().width === 0) layout = layout.parentElement;
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
      side.style.left = `${left}px`;
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
  // The drawer goes on the body, where no card's backdrop filter can pin it inside the card; the anchor marks the page it serves.
  return (
    <span ref={anchor} hidden>
      {createPortal(
        <aside ref={ref} className="side-drawer">
          <button type="button" className="sidebar-backdrop" aria-label="Close options" tabIndex={-1} onClick={onClose} />
          {children}
        </aside>,
        document.body,
      )}
    </span>
  );
}

/** OptionsToggle opens and closes a page's drawer, marked Active while any option differs from its default. */
export function OptionsToggle({ title, active, open, onClick }: { title: string; active: boolean; open: boolean; onClick: () => void }) {
  return (
    <button type="button" className="btn-antique options-toggle" title={title} aria-expanded={open} onClick={onClick}>
      <span>⚙️ Options</span>
      {active && <span className="status status-gold">Active</span>}
      <span className="options-toggle-arrow">{open ? '✕' : '▶'}</span>
    </button>
  );
}

interface PanelProps {
  /** What the drawer holds, for screen readers. */
  label: string;
  onClose: () => void;
  /** Where Reset All goes, or what it does; no Reset All without it. */
  reset?: Link | (() => void);
  /** A line above the footer's buttons, saying what is chosen. */
  summary?: ReactNode;
  children: ReactNode;
}

/** OptionsPanel is a page's options drawer with its title, Close, sections, and a footer with Reset All and Close. */
export function OptionsPanel({ label, onClose, reset, summary, children }: PanelProps) {
  return (
    <SideDrawer onClose={onClose}>
      <div className="options-panel" role="dialog" aria-label={label}>
        <div className="options-header">
          <span className="options-header-title">⚙️ Options &amp; Filters</span>
          <button type="button" className="btn-quiet" title="Close options panel" onClick={onClose}>
            ✕ Close
          </button>
        </div>
        {children}
        <div className="options-footer">
          {summary !== undefined && <div>{summary}</div>}
          <div className="options-footer-actions">
            {typeof reset === 'function' ? (
              <button type="button" className="btn-quiet options-reset" onClick={reset}>
                Reset All
              </button>
            ) : (
              reset !== undefined && (
                <a {...reset} className="btn-quiet options-reset">
                  Reset All
                </a>
              )
            )}
            <button type="button" className="btn-quiet" title="Close options panel" onClick={onClose}>
              ✕ Close
            </button>
          </div>
        </div>
      </div>
    </SideDrawer>
  );
}

interface SectionProps {
  title: ReactNode;
  icon?: string;
  /** Explains the section on hover. */
  tip?: string;
  /** The one field the title labels, when the section is a single field. */
  htmlFor?: string;
  /** Beside the title, at its right. */
  aside?: ReactNode;
  children: ReactNode;
}

/** OptionsSection is one titled group of choices in an OptionsPanel. */
export function OptionsSection({ title, icon, tip, htmlFor, aside, children }: SectionProps) {
  const heading = (
    <>
      {icon !== undefined && <span aria-hidden="true">{icon}</span>}
      <span>{title}</span>
      {aside !== undefined && <span className="options-group-aside">{aside}</span>}
    </>
  );
  return (
    <div className="options-section">
      {htmlFor === undefined ? (
        <div className="options-group-title" title={tip}>
          {heading}
        </div>
      ) : (
        <label className="options-group-title" htmlFor={htmlFor} title={tip}>
          {heading}
        </label>
      )}
      {children}
    </div>
  );
}

/** OptionsDivider heads the sections after it, as a part of the drawer about one thing. */
export function OptionsDivider({ children }: { children: ReactNode }) {
  return <div className="options-section-divider">{children}</div>;
}
