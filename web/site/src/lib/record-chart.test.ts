import { describe, expect, it } from 'vitest';
import { notCounted, recordChart } from './record-chart';

const step = (at: string, domination: number) => ({ entered_at: at, domination });

describe('recordChart', () => {
  const chart = recordChart([step('2024-03-01T00:00:00Z', 40), step('2025-06-01T00:00:00Z', 90)], Date.parse('2026-03-01T00:00:00Z'));

  it('runs the line flat from each record to the next, then on to now', () => {
    const [first, second] = chart.points;
    expect(chart.path).toBe(`M${first.x},${first.y}H${second.x}V${second.y}H${chart.right}`);
  });

  it('puts a higher record higher', () => {
    expect(chart.points[1].y).toBeLessThan(chart.points[0].y);
  });

  it('ticks the values from 0 past the top, and each new year', () => {
    expect(chart.yTicks.map((t) => t.value)).toEqual([0, 25, 50, 75, 100]);
    expect(chart.xTicks.map((t) => t.year)).toEqual([2025, 2026]);
  });
});

describe('notCounted', () => {
  const entries = [
    { domination: 50, proof_url: 'p', bugged: true },
    { domination: 900, proof_url: null, bugged: true },
    { domination: 70, proof_url: 'p', bugged: false },
  ];
  it.each([
    ['verified only: bugged entries with a proof', true, [50]],
    ['all entries: every bugged one, highest first', false, [900, 50]],
  ])('lists %s', (_name, verifiedOnly, want) => {
    expect(notCounted(entries, verifiedOnly).map((e) => e.domination)).toEqual(want);
  });
});
