/** A long table's cut: its first rows, and the rest behind a Show all button. */

/** How many rows a long table shows before its Show all button. */
export const ROWS_SHOWN = 10;

/** cutsRows says whether a table of count rows is cut, and so has a Show all button. */
export function cutsRows(count: number): boolean {
  return count > ROWS_SHOWN;
}

/** shownRows is the rows a long table shows: every row once expanded or when it is not cut, otherwise the first ROWS_SHOWN. */
export function shownRows<T>(rows: readonly T[], expanded: boolean): T[] {
  return expanded || !cutsRows(rows.length) ? [...rows] : rows.slice(0, ROWS_SHOWN);
}
