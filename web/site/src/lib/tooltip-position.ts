/** Where upstream's floating tooltip goes: above its target, or below, and always inside the viewport. */

interface Rect {
  left: number;
  top: number;
  width: number;
  bottom: number;
}

const pad = 12;
const gap = 8;

/** placeTooltip returns the box's fixed position for a target, the box's size and the viewport. */
export function placeTooltip(
  target: Rect,
  box: { width: number; height: number },
  viewport: { width: number; height: number },
  below: boolean,
): { left: number; top: number } {
  let top = target.top - box.height - gap;
  let left = target.left + (target.width - box.width) / 2;
  if (top < pad || below) top = target.bottom + gap;
  left = Math.min(Math.max(left, pad), viewport.width - pad - box.width);
  top = Math.min(Math.max(top, pad), viewport.height - pad - box.height);
  return { left: Math.round(left), top: Math.round(top) };
}
