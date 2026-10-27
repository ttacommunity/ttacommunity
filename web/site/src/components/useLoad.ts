/** A hook that loads data for a page and reloads it when what it reads changes. */
import { useEffect, useState } from 'react';
import { errorText, type Load } from '@ttac/shared/load';

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
