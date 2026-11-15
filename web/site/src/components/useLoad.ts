/** Hooks that load data for a page and reload it when what it reads changes. */
import { useEffect, useState } from 'react';
import { errorText, failed, shownLoad, type Found, type Load } from '@ttac/shared/load';

/** useLoad runs load whenever key, which names everything load reads, changes; a null load waits. */
export function useLoad<T>(key: string, load: (() => Promise<T>) | null): Load<T> {
  return useRequest<T, Load<T>>(key, load, (e) => ({ kind: 'error', message: errorText(e) }));
}

/** useFound loads as useLoad does something the address names: missing when the API answers 404. */
export function useFound<T>(key: string, load: (() => Promise<T>) | null): Found<T> {
  return useRequest<T, Found<T>>(key, load, failed);
}

/** useRequest runs load whenever key changes, ignoring answers to an older key, and says a failure as failure does. */
function useRequest<T, S extends Found<T>>(key: string, load: (() => Promise<T>) | null, failure: (e: unknown) => S): S {
  const [state, setState] = useState<Found<T>>({ kind: 'loading' });
  useEffect(() => {
    if (load === null) return;
    let stale = false;
    setState({ kind: 'loading' });
    load()
      .then((data) => !stale && setState({ kind: 'ready', data }))
      .catch((e: unknown) => !stale && setState(failure(e)));
    return () => {
      stale = true;
    };
  }, [key, load === null]);
  return state as S;
}

/** useKeptLoad loads as useLoad does, but while it loads again shows the last data, so a switch keeps the page's height and scroll. */
export function useKeptLoad<T>(key: string, load: (() => Promise<T>) | null): Load<T> {
  return useKept(useLoad(key, load));
}

/** useKeptFound loads as useFound does, keeping the last data while it loads again. */
export function useKeptFound<T>(key: string, load: (() => Promise<T>) | null): Found<T> {
  return useKept(useFound(key, load));
}

/** useKept is next, or while it loads again the last ready state. */
function useKept<S extends Found<unknown>>(next: S): S {
  const [last, setLast] = useState<S | null>(null);
  useEffect(() => {
    if (next.kind === 'ready') setLast(next);
  }, [next]);
  return shownLoad(last, next);
}
