import { describe, expect, it } from 'vitest';
import { podiumNames, podiumRequest, podiumStart } from './podium';

describe('podiumRequest', () => {
  it('sends the places filled in, trimmed, with their place numbers', () => {
    expect(podiumRequest('2025', [' alder440 ', '', 'Eider'], ' From CGE ')).toEqual({
      season: '2025',
      podium: [
        { place: 1, name: 'alder440' },
        { place: 3, name: 'Eider' },
      ],
      note: 'From CGE',
    });
  });

  it('gives null when no place is filled in', () => {
    expect(podiumRequest('2025', ['', ' ', ''], 'x')).toBeNull();
  });
});

describe('podiumNames', () => {
  it('lays a saved podium out as three boxes, empty where a place is not set', () => {
    expect(podiumNames([{ place: 3, name: 'Eider' }, { place: 1, name: 'alder440' }])).toEqual(['alder440', '', 'Eider']);
    expect(podiumNames([])).toEqual(['', '', '']);
  });
});

describe('podiumStart', () => {
  it.each([
    ['a podium set by hand', [{ place: 1, name: 'alder440' }], [{ place: 1, name: 'birch550' }], 'recorded', ['alder440', '', ''], 'admin'],
    ['else the suggestion', [], [{ place: 1, name: 'birch550' }, { place: 2, name: 'cedar660' }], 'final_stage', ['birch550', 'cedar660', ''], 'final_stage'],
    ['a tie fills the boxes in order', [], [{ place: 1, name: 'birch550' }, { place: 1, name: 'cedar660' }, { place: 3, name: 'elm770' }], 'final_stage', ['birch550', 'cedar660', 'elm770'], 'final_stage'],
    ['else nothing', [], [], '', ['', '', ''], 'none'],
  ] as const)('starts from %s', (_, saved, suggested, from, names, origin) => {
    expect(podiumStart([...saved], [...suggested], from)).toEqual({ names, origin });
  });
});
