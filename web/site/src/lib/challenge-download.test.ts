import { describe, expect, it } from 'vitest';
import { downloadName, downloadRows, toCsv } from './challenge-download';

const challenges = [{ id: 'epic1_exp', set: 'expansion' as const, group: 'epic', name: 'Long Road', players: 3 }];
const entry = { id: 7, challenge: 'epic1_exp', player: 'Ann', domination: 120, proof_url: 'https://discord.com/channels/1/2/3', entered_at: '2025-03-01T10:00:00Z', bugged: false };

describe('downloadRows', () => {
  it('writes each entry with its challenge in words', () => {
    expect(downloadRows([entry], challenges)[0]).toEqual({
      id: 7,
      date: '2025-03-01T10:00:00Z',
      player: 'Ann',
      set: 'Expansion',
      group: 'Epic Game',
      challenge_id: 'epic1_exp',
      challenge: 'Long Road',
      players: 3,
      domination: 120,
      verified: 'yes',
      proof_url: 'https://discord.com/channels/1/2/3',
      bugged: 'no',
    });
  });
  it('leaves out an entry of a challenge it does not know', () => {
    expect(downloadRows([{ ...entry, challenge: 'gone' }], challenges)).toEqual([]);
  });
});

describe('toCsv', () => {
  const row = downloadRows([entry], challenges)[0];
  it('starts with a byte-order mark and a header, a line per row', () => {
    const csv = toCsv([row]);
    expect(csv.startsWith('﻿id,date,player,')).toBe(true);
    expect(csv.split('\r\n')).toHaveLength(2);
  });
  it.each([
    ['quotes a comma', 'A, B', '"A, B"'],
    ['doubles a quote', 'say "hi"', '"say ""hi"""'],
    ['keeps a name that starts like a formula from running', '=HYPERLINK("x")', `"'=HYPERLINK(""x"")"`],
    ['and one starting with a plus', '+1', "'+1"],
  ])('%s', (_name, player, cell) => {
    expect(toCsv([{ ...row, player }]).split('\r\n')[1]).toContain(`,${cell},`);
  });
  it('leaves a negative domination a number', () => {
    expect(toCsv([{ ...row, domination: -20 }])).toContain(',-20,');
  });
});

describe('downloadName', () => {
  it("names the file by the day's date", () => {
    expect(downloadName(new Date('2026-10-29T12:00:00Z'), 'csv')).toBe('tta-challenge-dominations-2026-10-29.csv');
  });
});
