/** A sortable table's column header, the same on the site and in admin, and the state that holds its sort. */
import { useState, type ReactNode } from 'react';
import { nextSort, sortRows, type Sort, type SortColumns } from './sort';

/** useSort holds a table's sort, off until a header is chosen: the rows in that order, the sort, and a function choosing a column. */
export function useSort<T, C extends string>(rows: readonly T[], columns: SortColumns<T, C>): [T[], Sort<C>, (col: C) => void] {
  const [sort, setSort] = useState<Sort<C>>(null);
  return [sortRows(rows, columns, sort), sort, (col) => setSort((s) => nextSort(s, col, columns))];
}

interface Props<C extends string> {
  col: C;
  sort: Sort<C>;
  onSort: (col: C) => void;
  className?: string;
  children: ReactNode;
}

/** SortHeader is a column header whose button sorts by its column; the arrow and aria-sort say which way. */
export default function SortHeader<C extends string>({ col, sort, onSort, className, children }: Props<C>) {
  const way = sort?.col === col ? (sort.desc ? 'descending' : 'ascending') : undefined;
  return (
    <th scope="col" className={className} aria-sort={way}>
      <SortButton col={col} sort={sort} onSort={onSort}>
        {children}
      </SortButton>
    </th>
  );
}

/** SortButton sorts by its column, its arrow saying which way: a header's button, or one of several sharing a header (W - L - D). */
export function SortButton<C extends string>({ col, sort, onSort, className, children }: Props<C>) {
  const on = sort?.col === col;
  return (
    <button type="button" className={`sort-header${on ? ' sort-on' : ''}${className === undefined ? '' : ` ${className}`}`} onClick={() => onSort(col)}>
      {children}
      <span className="sort-arrow" aria-hidden="true">
        {!on ? '⇅' : sort.desc ? '▼' : '▲'}
      </span>
    </button>
  );
}
