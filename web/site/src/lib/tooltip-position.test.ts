import { describe, expect, it } from 'vitest';
import { placeTooltip } from './tooltip-position';

const viewport = { width: 1000, height: 800 };
const size = { width: 200, height: 40 };
const target = (left: number, top: number, width = 100, height = 20) => ({ left, top, width, height, right: left + width, bottom: top + height });

describe('placeTooltip', () => {
  it('centres the box above its target', () => {
    expect(placeTooltip(target(400, 300), size, viewport, false)).toEqual({ left: 350, top: 252 });
  });

  it('opens below when there is no room above, or when asked to', () => {
    expect(placeTooltip(target(400, 20), size, viewport, false)).toEqual({ left: 350, top: 48 });
    expect(placeTooltip(target(400, 300), size, viewport, true)).toEqual({ left: 350, top: 328 });
  });

  it('keeps 12px from the left and right edges', () => {
    expect(placeTooltip(target(0, 300, 20), size, viewport, false).left).toBe(12);
    expect(placeTooltip(target(980, 300, 20), size, viewport, false).left).toBe(788);
  });

  it('keeps 12px from the bottom edge', () => {
    expect(placeTooltip(target(400, 790), size, viewport, true).top).toBe(748);
  });
});
