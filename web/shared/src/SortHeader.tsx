/** A sortable table's column header, the same on the site and in admin, and the state that holds its sort. */
import { useState, type MouseEvent, type ReactNode } from 'react';
import { nextSort, sortArrow, sortRows, type Sort, type SortColumns, type SortWay } from './sort';

/** useSort holds a table's sort, off until a header is chosen: the rows in that order, the sort, and a function choosing a column. */
export function useSort<T, C extends string>(rows: readonly T[], columns: SortColumns<T, C>): [T[], Sort<C>, (col: C) => void] {
  const [sort, setSort] = useState<Sort<C>>(null);
  return [sortRows(rows, columns, sort), sort, (col) => setSort((s) => nextSort(s, col, columns))];
}

interface Props<C extends string> {
  col: C;
  sort: Sort<C>;
  onSort: (col: C) => void;
  children: ReactNode;
}

/** Where a column's header and cells sit: end is for numbers, which also line up their digits. */
export type Align = 'start' | 'center' | 'end';

const ALIGN_CLASS: Record<Align, string> = { start: 'col-start', center: 'col-center', end: 'num' };

/** alignClass is the class that sits a header or cell where align says, for a table's plain cells beside its SortHeaders. */
export function alignClass(align: Align | undefined): string | undefined {
  return align === undefined ? undefined : ALIGN_CLASS[align];
}

/** SortHeader is a column header whose button sorts by its column; the arrow and aria-sort say which way, align where the column sits. */
export default function SortHeader<C extends string>({ col, sort, onSort, align, children }: Props<C> & { align?: Align }) {
  const way = sort?.col === col ? (sort.desc ? 'descending' : 'ascending') : undefined;
  return (
    <th scope="col" className={alignClass(align)} aria-sort={way}>
      <SortButton col={col} sort={sort} onSort={onSort}>
        {children}
      </SortButton>
    </th>
  );
}

/** SortButton sorts by its column, its arrow saying which way: a header's button, or one of several sharing a header (W - L - D), each in its result's tone. */
export function SortButton<C extends string>({ col, sort, onSort, tone, children }: Props<C> & { tone?: 'win' | 'loss' | 'draw' }) {
  const on = sort?.col === col;
  return (
    <button type="button" className={`sort-header${on ? ' sort-on' : ''}${tone === undefined ? '' : ` sort-header-${tone}`}`} onClick={() => onSort(col)}>
      {children}
      <span className="sort-arrow" aria-hidden="true">
        {sortArrow(on ? (sort.desc ? 'descending' : 'ascending') : undefined)}
      </span>
    </button>
  );
}

/** SortLink is a header's sort as a link, for a page whose sort is in its address: link goes to the table sorted by this column, way says how it sorts now (its th's aria-sort), hint explains the column under the mouse. */
export function SortLink({ link, way, hint, children }: { link: { href: string; onClick: (e: MouseEvent<HTMLAnchorElement>) => void }; way: SortWay; hint?: string; children: ReactNode }) {
  return (
    <a className={way === undefined ? 'sort-header' : 'sort-header sort-on'} href={link.href} onClick={link.onClick} data-tooltip={hint}>
      {children}
      <span className="sort-arrow" aria-hidden="true">
        {sortArrow(way)}
      </span>
    </a>
  );
}
