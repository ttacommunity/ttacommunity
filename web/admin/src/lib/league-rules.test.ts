import { describe, expect, it } from 'vitest';
import { recordGames, tiersChanged, tiersList, tiersText } from './league-rules';

describe('tiersList and tiersText', () => {
  it.each([
    ['one pair', 'Grandmaster 1', [{ name: 'Grandmaster', tier: 1 }]],
    ['pairs in order, spaces around', ' Emperor 1 , King 2,Prince 3 ', [{ name: 'Emperor', tier: 1 }, { name: 'King', tier: 2 }, { name: 'Prince', tier: 3 }]],
    ['a name of several words', 'Second Division 2', [{ name: 'Second Division', tier: 2 }]],
    ['blanks between commas', 'Gold 4,, Silver 5', [{ name: 'Gold', tier: 4 }, { name: 'Silver', tier: 5 }]],
    ['an empty box', '  ', []],
  ])('read a league\'s divisions from %s', (_name, box, want) => {
    expect(tiersList(box)).toEqual(want);
  });

  it.each([
    ['a tier above 7', 'Gold 8'],
    ['a tier below 1', 'Gold 0'],
    ['no tier', 'Gold'],
    ['a decimal tier', 'Gold 1.5'],
    ['a division listed twice', 'Gold 1, Gold 2'],
  ])('refuse %s, as the API would', (_name, box) => {
    expect(tiersList(box)).toBeNull();
  });

  it('write divisions for the box it reads', () => {
    const divisions = [{ name: 'Emperor', tier: 1 }, { name: 'Baron', tier: 7 }];
    expect(tiersList(tiersText(divisions))).toEqual(divisions);
  });

  it.each([
    ['the same, spaced differently', ' Emperor 1,King 2 ', false],
    ['another tier', 'Emperor 1, King 3', true],
    ['a refused box', 'Emperor 9', false],
  ])('say whether a box changes the saved divisions: %s', (_name, box, want) => {
    expect(tiersChanged(box, [{ name: 'Emperor', tier: 1 }, { name: 'King', tier: 2 }])).toBe(want);
  });
});

describe('recordGames', () => {
  it.each([
    ['a number', '6', 6],
    ['zero', '0', 0],
    ['spaces around', ' 5 ', 5],
    ['a negative number', '-1', null],
    ['over 100', '101', null],
    ['a decimal', '5.5', null],
    ['an empty box', '', null],
  ])('read the fewest games for a season record from %s', (_name, box, want) => {
    expect(recordGames(box)).toBe(want);
  });
});
