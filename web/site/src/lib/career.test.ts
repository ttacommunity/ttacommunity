import { describe, expect, it } from 'vitest';
import type { CareerYear, VersusMatch } from '@ttac/shared/ratings';
import { careerPeakYear, isPeakHigher, scoreYears, versusTally, wcLabel } from './career';

const year = (y: number, wins: number, winRate: number, peak: number | null, title: CareerYear['title'] = null): CareerYear => ({
  year: y,
  opponents: wins * 2,
  wins,
  losses: wins,
  draws: 0,
  win_rate: winRate,
  peak_rating: peak,
  title,
  year_end_rating: null,
  career_opponents: wins * 2,
  career_wins: wins,
  career_losses: wins,
  career_draws: 0,
  career_win_rate: winRate,
});

describe('careerPeakYear', () => {
  it("scores years as upstream: wins, win rate, peak over 1500 and the year's title", () => {
    // 2023: 10·1.5 + 50·2 + 100·0.4 + GM 60 = 215; 2024: 20·1.5 + 40·2 + 50·0.4 + 0 = 130
    const got = careerPeakYear([year(2023, 10, 50, 1600, 'GM'), year(2024, 20, 40, 1550)], null);
    expect(got).toMatchObject({ year: 2023, score: 215, title: 'GM' });
  });

  it('adds 30 in the peak title year, and a missing peak adds nothing', () => {
    const years = [year(2023, 10, 50, null), year(2024, 10, 50, null)];
    expect(careerPeakYear(years, { code: 'M', year: 2024 })?.year).toBe(2024);
    expect(careerPeakYear(years, null)?.score).toBe(115);
  });

  it('keeps the first of equal years', () => {
    expect(careerPeakYear([year(2023, 10, 50, 1600), year(2024, 10, 50, 1600)], null)?.year).toBe(2023);
  });

  it("falls back to the career peak's title when its year has none", () => {
    expect(careerPeakYear([year(2023, 10, 50, 1600)], { code: 'WC', year: 2019 })?.title).toBe('WC');
  });

  it('is null without years', () => {
    expect(careerPeakYear([], null)).toBeNull();
  });
});

describe('isPeakHigher', () => {
  it('is true when the career peak outranks the current title, or there is none', () => {
    expect(isPeakHigher('M', { code: 'GM', year: 2022 })).toBe(true);
    expect(isPeakHigher(null, { code: 'W', year: 2022 })).toBe(true);
    expect(isPeakHigher('GM', { code: 'GM', year: 2022 })).toBe(false);
    expect(isPeakHigher('M', { code: 'GM', year: null })).toBe(false);
    expect(isPeakHigher('M', null)).toBe(false);
  });
});

const game = (format: number, namePlace: number, otherPlace: number): VersusMatch => ({
  match_id: namePlace * 10 + otherPlace,
  tournament: 'Cup',
  date: '2026-01-01',
  player_count: format,
  name_place: namePlace,
  other_place: otherPlace,
  seats: [],
});

describe('versusTally', () => {
  const games = [game(2, 1, 2), game(3, 3, 1), game(3, 2, 2), game(4, 1, 4)];

  it('counts who finished ahead in every format, or one', () => {
    expect(versusTally(games, 0)).toMatchObject({ games: 4, nameWins: 2, otherWins: 1, draws: 1 });
    expect(versusTally(games, 3)).toMatchObject({ games: 2, nameWins: 0, otherWins: 1, draws: 1 });
    expect(versusTally(games, 3).rows.map((m) => m.player_count)).toEqual([3, 3]);
  });
});

describe('scoreYears', () => {
  it("scores every year, keeping each year's own title", () => {
    const got = scoreYears([year(2023, 10, 50, 1600, 'GM'), year(2024, 20, 40, 1550)], { code: 'WC', year: 2024 });
    expect(got.map((y) => [y.year, y.score, y.title])).toEqual([
      [2023, 215, 'GM'],
      [2024, 160, null],
    ]);
  });

  it('gives a Super GM year its bonus of 100', () => {
    expect(scoreYears([year(2023, 10, 50, 1600, 'SGM')], null).map((y) => y.score)).toEqual([255]);
  });
});

describe('wcLabel', () => {
  it("names the reigning champion, then a past champion's year, as upstream's header", () => {
    expect(wcLabel({ code: 'WC', count: 1, reason: '' }, { code: 'WC', year: 2025 })).toBe('Reigning WC');
    expect(wcLabel({ code: 'GM', count: 1, reason: '' }, { code: 'WC', year: 2024 })).toBe('2024 WC');
    expect(wcLabel(null, { code: 'WC', year: 2023 })).toBe('2023 WC');
    expect(wcLabel({ code: 'GM', count: 1, reason: '' }, { code: 'GM', year: 2022 })).toBeNull();
    expect(wcLabel(null, null)).toBeNull();
  });
});

describe('careerPeakYear ties', () => {
  it('compares the unrounded scores, as upstream', () => {
    // 2023 scores 100.04 and 2024 100.01: both round to 100.0, and 2023 is higher.
    const a = { ...year(2023, 0, 50.02, null) };
    const b = { ...year(2024, 0, 50.005, null) };
    expect(careerPeakYear([b, a], null)?.year).toBe(2023);
  });
});
