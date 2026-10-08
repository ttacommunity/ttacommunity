import { describe, expect, it } from 'vitest';
import { podiumNames, podiumRequest } from './podium';

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
