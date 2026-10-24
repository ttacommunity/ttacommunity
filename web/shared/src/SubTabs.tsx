/** A page's sub tabs, the same on the site and in admin: upstream's records tabs choosing which panel shows. */
import { useEffect, useState, type ReactNode } from 'react';
import Segmented from './Segmented';
import { shownTab } from './tabs';

/** useHash follows the address's hash ("#pairings"), so a link can open a tab and back returns to the last one: the hash ('' until mounted) and a function that sets it. */
export function useHash(): [string, (value: string) => void] {
  const [hash, setHash] = useState('');
  useEffect(() => {
    const follow = () => setHash(window.location.hash);
    follow();
    window.addEventListener('hashchange', follow);
    return () => window.removeEventListener('hashchange', follow);
  }, []);
  const choose = (value: string) => {
    if (window.location.hash !== `#${value}`) history.pushState(null, '', `#${value}`);
    setHash(`#${value}`);
  };
  return [hash, choose];
}

export interface Tab<T> {
  value: T;
  label: ReactNode;
  panel: ReactNode;
}

interface Props<T> {
  label: string;
  tabs: Tab<T>[];
  /** The tab last chosen, or null for the first. */
  chosen: T | null;
  onChoose: (value: T) => void;
}

/** SubTabs shows the tab bar and the shown tab's panel. Every panel stays mounted, so what is half done in one is kept while another is shown. */
export default function SubTabs<T extends string>({ label, tabs, chosen, onChoose }: Props<T>) {
  const shown = shownTab(
    tabs.map((t) => t.value),
    chosen,
  );
  return (
    <>
      <div className="sub-tabs">
        <Segmented label={label} look="records" options={tabs.map(({ value, label: text }) => ({ value, label: text }))} value={shown} onChange={onChoose} />
      </div>
      {tabs.map((t) => (
        <div key={t.value} hidden={t.value !== shown}>
          {t.panel}
        </div>
      ))}
    </>
  );
}
