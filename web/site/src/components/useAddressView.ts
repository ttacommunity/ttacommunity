/** useAddressView keeps a page's view in its address, so a view can be shared: read on load and on Back, written and remembered on each change. */
import { useCallback, useEffect, useState } from 'react';
import { browserStorage } from '@ttac/shared/storage';
import type { Remembered, View } from '../lib/leaderboard-view';
import { viewLinks, type Link } from '../lib/navigate';
import { recall, remember } from '../lib/remembered';

/** A page's address: each function defined once, outside the component. */
export interface Address<V> {
  /** parse reads a view from the address, falling back to the remembered choices. */
  parse: (params: URLSearchParams, remembered: Remembered) => V;
  /** params writes a view's address. */
  params: (view: V) => URLSearchParams;
  /** remembers is the leaderboard view whose engine, reset mode and format a change remembers. */
  remembers: (view: V) => View;
}

/** readAddress reads the page's view from the address it is at. */
export function readAddress<V>(address: Address<V>): V {
  return address.parse(new URLSearchParams(window.location.search), recall(browserStorage()));
}

/** useAddressView returns the view, go (show another view, a new history entry), replace (correct the address in place) and linkTo (a link to another view that changes it without a reload). */
export function useAddressView<V>(address: Address<V>) {
  const [view, setView] = useState<V>(() => readAddress(address));
  const go = useCallback(
    (next: V) => {
      window.history.pushState(null, '', `?${address.params(next)}`);
      remember(browserStorage(), address.remembers(next));
      setView(next);
    },
    [address],
  );
  const replace = useCallback(
    (next: V) => {
      window.history.replaceState(null, '', `?${address.params(next)}`);
      setView(next);
    },
    [address],
  );
  useEffect(() => {
    const onPop = () => setView(readAddress(address));
    window.addEventListener('popstate', onPop);
    return () => window.removeEventListener('popstate', onPop);
  }, [address]);
  const linkTo: (next: V) => Link = viewLinks(address.params, go);
  return { view, go, replace, linkTo };
}
