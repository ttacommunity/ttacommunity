/** Rating tiers shaded behind a chart: which tiers a rating range meets, and drawing them on either axis. */
import { RATING_TIERS, type RatingTier } from './player-view';

/** One tier's stretch of a chart's rating axis, in the axis's own values. */
export interface TierSpan {
  tier: RatingTier;
  start: number;
  end: number;
}

/** tierSpans is each tier the ratings lo to hi meet, lowest first, with its edges placed by at; the lowest and highest tiers have no bound. */
export function tierSpans(lo: number, hi: number, at: (rating: number) => number): TierSpan[] {
  return RATING_TIERS.flatMap((tier, i) => {
    const next = RATING_TIERS[i + 1];
    const from = Math.max(i === 0 ? -Infinity : tier.min, lo);
    const to = Math.min(next === undefined ? Infinity : next.min, hi);
    return from < to ? [{ tier, start: at(from), end: at(to) }] : [];
  });
}

/** The colours and font tier bands are drawn in, from the theme. */
export interface TierLook {
  line: string;
  text: string;
  font: string;
}

interface Area {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

/** drawTierBands shades spans along the rating axis (x across, y up), with a dashed line at each boundary and each tier's name, or its code when narrow, at the top of its band. */
export function drawTierBands(ctx: CanvasRenderingContext2D, area: Area, toPixel: (v: number) => number, axis: 'x' | 'y', spans: TierSpan[], look: TierLook) {
  ctx.save();
  ctx.beginPath();
  ctx.rect(area.left, area.top, area.right - area.left, area.bottom - area.top);
  ctx.clip();
  ctx.font = `11px ${look.font}`;
  ctx.lineWidth = 1;
  for (const [i, s] of spans.entries()) {
    const a = toPixel(s.start);
    const b = toPixel(s.end);
    const lo = Math.min(a, b);
    const size = Math.abs(b - a);
    ctx.fillStyle = s.tier.color;
    if (axis === 'x') ctx.fillRect(lo, area.top, size, area.bottom - area.top);
    else ctx.fillRect(area.left, lo, area.right - area.left, size);
    if (i > 0) {
      ctx.strokeStyle = s.tier.borderColor ?? look.line;
      ctx.setLineDash([4, 4]);
      ctx.beginPath();
      if (axis === 'x') {
        ctx.moveTo(a, area.top);
        ctx.lineTo(a, area.bottom);
      } else {
        ctx.moveTo(area.left, a);
        ctx.lineTo(area.right, a);
      }
      ctx.stroke();
    }
    ctx.fillStyle = look.text;
    const top = axis === 'x' ? area.top : Math.max(area.top, lo);
    const room = axis === 'x' ? size : area.right - area.left;
    const name = ctx.measureText(s.tier.name).width + 8 <= room ? s.tier.name : s.tier.code;
    const tall = axis === 'x' || Math.min(area.bottom, lo + size) - top >= 16;
    if (tall && ctx.measureText(name).width + 8 <= room) {
      ctx.textAlign = 'left';
      ctx.textBaseline = 'top';
      ctx.fillText(name, (axis === 'x' ? lo : area.left) + 4, top + 4);
    }
  }
  ctx.restore();
}
