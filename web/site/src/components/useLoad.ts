/** Hooks that load data for a page and reload it when what it reads changes. */
import { useEffect, useState } from 'react';
import { errorText, shownLoad, type Load } from '@ttac/shared/load';

/** useLoad runs load whenever key, which names everything load reads, changes; a null load waits. */
export function useLoad<T>(key: string, load: (() => Promise<T>) | null): Load<T> {
  const [state, setState] = useState<Load<T>>({ kind: 'loading' });
  useEffect(() => {
    if (load === null) return;
    let stale = false;
    setState({ kind: 'loading' });
    load()
      .then((data) => !stale && setState({ kind: 'ready', data }))
      .catch((e: unknown) => !stale && setState({ kind: 'error', message: errorText(e) }));
    return () => {
      stale = true;
    };
  }, [key, load === null]);
  return state;
}

/** useKeptLoad loads as useLoad does, but while it loads again shows the last data, so a switch keeps the page's height and scroll. */
export function useKeptLoad<T>(key: string, load: (() => Promise<T>) | null): Load<T> {
  const next = useLoad(key, load);
  const [last, setLast] = useState<Load<T> | null>(null);
  useEffect(() => {
    if (next.kind === 'ready') setLast(next);
  }, [next]);
  return shownLoad(last, next);
}
