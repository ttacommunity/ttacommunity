/** Which sides of a table's scroll box have more of the table beyond them. */

/** The sides of a scroll box with more beyond them. */
export interface Edges {
  start: boolean;
  end: boolean;
}

/** scrollEdges takes the box's scroll position, its visible width and its content's full width (pixels), and says which sides hide more; a pixel's rounding counts as none. */
export function scrollEdges(left: number, width: number, full: number): Edges {
  const hidden = full - width;
  if (hidden <= 1) return { start: false, end: false };
  return { start: left > 1, end: left < hidden - 1 };
}
