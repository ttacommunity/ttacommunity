/** Admin's header, in the site's style: brand, admin's tabs grouped in click-open menus, Community Hub, and the site's drawer on phones. */
import { useEffect, useRef, useState } from 'react';
import homeButton from '@ttac/shared/styles/img/home-button.webp';
import { groupsFor, siteUrlFor, type TABS, type TabKey } from '../lib/admin-tabs';

/** Count shows a number of waiting items beside a label, and says what it is to a screen reader. */
function Count({ n }: { n: number }) {
  if (n === 0) return null;
  return (
    <>
      <span className="count-badge" aria-hidden="true">
        {n}
      </span>
      <span className="visually-hidden">, {n} waiting</span>
    </>
  );
}

/** AdminHeader shows tabs in groupsFor's menus (current marked, counts beside them) linking to admin's home; a menu with one tab is a plain link. */
export default function AdminHeader({
  tabs,
  current,
  counts = {},
  siteUrl,
}: {
  tabs: typeof TABS;
  current: TabKey | null;
  counts?: Partial<Record<TabKey, number>>;
  siteUrl?: string;
}) {
  const [open, setOpen] = useState(false);
  const [menu, setMenu] = useState<string | null>(null);
  // The build has no host, so the hub starts as the production one and turns into this host's once in the browser.
  const [hub, setHub] = useState(siteUrl ?? siteUrlFor());
  useEffect(() => setHub(siteUrl ?? siteUrlFor(window.location.hostname)), [siteUrl]);
  const nav = useRef<HTMLElement>(null);
  const backdrop = useRef<HTMLDivElement>(null);
  const groups = groupsFor(tabs, counts);
  const here = groups.find((g) => g.tabs.some((t) => t.key === current))?.label ?? null;

  const setDrawer = (to: boolean) => {
    setOpen(to);
    setMenu(to ? here : null);
  };
  const close = () => {
    setOpen(false);
    setMenu(null);
  };

  useEffect(() => {
    // A click outside the drawer closes it, as on the site; Escape and the close button do for the keyboard.
    const el = backdrop.current;
    el?.addEventListener('click', close);
    return () => el?.removeEventListener('click', close);
  }, []);

  useEffect(() => {
    document.body.style.overflow = open ? 'hidden' : '';
    const button = () => nav.current?.querySelector<HTMLButtonElement>('.section-name[aria-expanded="true"]') ?? null;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented) return;
      if (open) close();
      else if (menu !== null) {
        button()?.focus();
        setMenu(null);
      }
    };
    // On desktop a menu closes when a click or the focus goes elsewhere; in the phone drawer it stays open.
    const leaves = (target: EventTarget | null) => {
      const b = button();
      return b !== null && !open && !(target instanceof Node && b.parentElement?.contains(target));
    };
    const onClick = (e: MouseEvent) => leaves(e.target) && setMenu(null);
    const onFocusOut = (e: FocusEvent) => e.relatedTarget !== null && leaves(e.relatedTarget) && setMenu(null);
    const el = nav.current;
    document.addEventListener('keydown', onKey);
    document.addEventListener('click', onClick);
    el?.addEventListener('focusout', onFocusOut);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('click', onClick);
      el?.removeEventListener('focusout', onFocusOut);
    };
  }, [open, menu]);

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
          onClick={() => setDrawer(!open)}
        >
          <span className="hamburger-bar"></span>
          <span className="hamburger-bar"></span>
          <span className="hamburger-bar"></span>
        </button>
        <div ref={backdrop} className={open ? 'nav-backdrop open' : 'nav-backdrop'}></div>
        <div id="main-nav" className={open ? 'nav-drawer site-menu open' : 'nav-drawer site-menu'}>
          <div className="nav-mobile-header">
            <span className="nav-mobile-title">Menu</span>
            <button type="button" className="nav-close-btn" aria-label="Close navigation" onClick={close}>
              ✕
            </button>
          </div>
          <nav ref={nav} className="sections" aria-label="Admin sections">
            <ul className="section-list">
              {groups.map((g, i) => {
                const isHere = g.label === here;
                if (g.tabs.length === 1) {
                  const t = g.tabs[0];
                  return (
                    <li key={g.label} className="section">
                      <a
                        href={`/#${t.key}`}
                        className={t.key === current ? 'section-name current' : 'section-name'}
                        aria-current={t.key === current ? 'page' : undefined}
                        onClick={close}
                      >
                        {t.label}
                        <Count n={counts[t.key] ?? 0} />
                      </a>
                    </li>
                  );
                }
                return (
                  <li key={g.label} className="section">
                    <button
                      type="button"
                      className={isHere ? 'section-name current' : 'section-name'}
                      aria-expanded={menu === g.label}
                      aria-controls={`admin-section-${i}`}
                      onClick={() => setMenu(menu === g.label ? null : g.label)}
                    >
                      {g.label}
                      {isHere && <span className="visually-hidden">(current section)</span>}
                      <Count n={g.count} />
                      <svg className="caret" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" aria-hidden="true">
                        <path d="M6 9l6 6 6-6"></path>
                      </svg>
                    </button>
                    <ul id={`admin-section-${i}`} className="section-pages">
                      {g.tabs.map((t) => (
                        <li key={t.key}>
                          <a
                            href={`/#${t.key}`}
                            className={t.key === current ? 'current' : undefined}
                            aria-current={t.key === current ? 'page' : undefined}
                            onClick={close}
                          >
                            <span className="page-label">
                              {t.label}
                              <Count n={counts[t.key] ?? 0} />
                            </span>
                          </a>
                        </li>
                      ))}
                    </ul>
                  </li>
                );
              })}
            </ul>
          </nav>
          <div className="header-controls">
            <a href={hub} onClick={close}>
              Community Hub
            </a>
          </div>
        </div>
      </div>
    </header>
  );
}
