/** Links between a page's views: the address changes without a reload, and modified clicks stay the browser's. */

/** Click is what a link's click handler reads: React's MouseEvent has it all. */
export interface Click {
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
  button: number;
  preventDefault(): void;
}

/** Link is an anchor's href and click handler. */
export type Link = { href: string; onClick: (e: Click) => void };

/** followsLink says whether the browser opens a click itself: with Cmd, Ctrl or Shift held, or a button other than the main one. */
export function followsLink(e: Click): boolean {
  return e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0;
}

/** viewLinks returns a link maker for a page's views: params writes a view's query, go shows it on a plain click. */
export function viewLinks<V>(params: (view: V) => URLSearchParams, go: (view: V) => void): (next: V) => Link {
  return (next) => ({
    href: `?${params(next)}`,
    onClick: (e) => {
      if (followsLink(e)) return;
      e.preventDefault();
      go(next);
    },
  });
}
