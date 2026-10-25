/** Sorting a table by one of its columns, for any table on either site: off (the table's own order), its first way, then the other. */

/** SortValue is what a column sorts a row by; null sorts last either way, and an array sorts by each value in turn. */
export type SortValue = string | number | null;

/** SortColumn reads a row's value for a column, and says which way the column sorts first: names asc, numbers desc. */
export type SortColumn<T> = { value: (row: T) => SortValue | readonly SortValue[]; first: 'asc' | 'desc' };

/** SortColumns are a table's sortable columns, by id. */
export type SortColumns<T, C extends string> = Record<C, SortColumn<T>>;

/** Sort is the column a table is sorted by and which way, or null for the table's own order. */
export type Sort<C extends string> = { col: C; desc: boolean } | null;

/** nextSort is the sort after choosing col: its first way, then the other, then the table's own order again. */
export function nextSort<T, C extends string>(current: Sort<C>, col: C, columns: SortColumns<T, C>): Sort<C> {
  const first = columns[col].first === 'desc';
  if (current === null || current.col !== col) return { col, desc: first };
  return current.desc === first ? { col, desc: !first } : null;
}

const collator = new Intl.Collator('en', { sensitivity: 'base', numeric: true });

function compareOne(a: SortValue, b: SortValue, desc: boolean): number {
  if (a === null || b === null) return a === b ? 0 : a === null ? 1 : -1;
  const c = typeof a === 'number' && typeof b === 'number' ? a - b : collator.compare(String(a), String(b));
  return desc ? -c : c;
}

/** sortRows gives rows sorted by sort, rows level on its column keeping the table's own order; the rows as given when sort is null. */
export function sortRows<T, C extends string>(rows: readonly T[], columns: SortColumns<T, C>, sort: Sort<C>): T[] {
  if (sort === null) return [...rows];
  const { value } = columns[sort.col];
  const values = (r: T) => {
    const v = value(r);
    return Array.isArray(v) ? v : [v as SortValue];
  };
  return rows
    .map((row, i) => ({ row, i, v: values(row) }))
    .sort((x, y) => {
      for (let k = 0; k < Math.max(x.v.length, y.v.length); k++) {
        const c = compareOne(x.v[k] ?? null, y.v[k] ?? null, sort.desc);
        if (c !== 0) return c;
      }
      return x.i - y.i;
    })
    .map(({ row }) => row);
}
