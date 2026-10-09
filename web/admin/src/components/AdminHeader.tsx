/** Admin's header, the site's: brand, then admin's sections as its links, and the site's menu on phones. */
import { useEffect, useRef, useState } from 'react';
import homeButton from '@ttac/shared/styles/img/home-button.webp';
import type { TABS, TabKey } from '../lib/admin-tabs';

/** AdminHeader links each of tabs (current marked) to admin's home; on phones they open from the menu button. */
export default function AdminHeader({ tabs, current }: { tabs: typeof TABS; current: TabKey | null }) {
  const [open, setOpen] = useState(false);
  const backdrop = useRef<HTMLDivElement>(null);
  useEffect(() => {
    // A click outside the menu closes it, as on the site; Escape and the close button do for the keyboard.
    const el = backdrop.current;
    const close = () => setOpen(false);
    el?.addEventListener('click', close);
    return () => el?.removeEventListener('click', close);
  }, []);
  useEffect(() => {
    document.body.style.overflow = open ? 'hidden' : '';
    const close = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    document.addEventListener('keydown', close);
    return () => document.removeEventListener('keydown', close);
  }, [open]);
  return (
    <header className="site-header">
      <div className="header-container">
        <a href="/" className="brand">
          <img src={homeButton.src} alt="" width="42" height="42" />
          <div>
            <span className="brand-title">Through the Ages</span>
            <span className="brand-subtitle">Community admin</span>
          </div>
        </a>
        <button
          type="button"
          className={open ? 'nav-toggle open' : 'nav-toggle'}
          aria-label="Toggle navigation"
          aria-expanded={open}
          aria-controls="main-nav"
          onClick={() => setOpen(!open)}
        >
          <span className="hamburger-bar"></span>
          <span className="hamburger-bar"></span>
          <span className="hamburger-bar"></span>
        </button>
        <div ref={backdrop} className={open ? 'nav-backdrop open' : 'nav-backdrop'}></div>
        <nav id="main-nav" aria-label="Admin sections" className={open ? 'open' : undefined}>
          <div className="nav-mobile-header">
            <span className="nav-mobile-title">Navigation</span>
            <button type="button" className="nav-close-btn" aria-label="Close navigation" onClick={() => setOpen(false)}>
              ✕
            </button>
          </div>
          {tabs.map((t) => (
            <a
              key={t.key}
              href={`/#${t.key}`}
              className={t.key === current ? 'active' : undefined}
              aria-current={t.key === current ? 'page' : undefined}
              onClick={() => setOpen(false)}
            >
              {t.label}
            </a>
          ))}
        </nav>
      </div>
    </header>
  );
}
