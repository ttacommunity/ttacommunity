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
      <button type="button" className="sort-header" onClick={() => onSort(col)}>
        {children}
        <span className="sort-arrow" aria-hidden="true">
          {way === undefined ? '⇅' : way === 'ascending' ? '▲' : '▼'}
        </span>
      </button>
    </th>
  );
}
