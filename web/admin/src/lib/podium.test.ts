import { describe, expect, it } from 'vitest';
import { podiumNames, podiumNoteStart, podiumRequest, podiumStart, sharePlace } from './podium';

describe('podiumRequest', () => {
  it.each([
    ['the places filled in, trimmed, with their place numbers', [[' alder440 '], [''], ['Eider']], ' From CGE ',
      { podium: [{ place: 1, name: 'alder440' }, { place: 3, name: 'Eider' }], note: 'From CGE' }],
    ['every player sharing a place, with that place', [['alder440'], ['birch550', ' ', 'cedar660'], ['']], '',
      { podium: [{ place: 1, name: 'alder440' }, { place: 2, name: 'birch550' }, { place: 2, name: 'cedar660' }], note: '' }],
    ['null when no place is filled in', [[''], [' '], ['', '']], 'x', null],
  ] as const)('sends %s', (_, places, note, want) => {
    expect(podiumRequest(places.map((p) => [...p]), note)).toEqual(want);
  });
});

describe('podiumNames', () => {
  it.each([
    ['a box for each place, empty where a place is not set', [{ place: 3, name: 'Eider' }, { place: 1, name: 'alder440' }], [['alder440'], [''], ['Eider']]],
    ['a box for each player sharing a place', [{ place: 1, name: 'alder440' }, { place: 2, name: 'birch550' }, { place: 2, name: 'cedar660' }], [['alder440'], ['birch550', 'cedar660'], ['']]],
    ['three empty boxes for none', [], [[''], [''], ['']]],
  ] as const)('lays out %s', (_, podium, want) => {
    expect(podiumNames([...podium])).toEqual(want);
  });
});

describe('podiumStart', () => {
  it.each([
    ['a podium set by hand', [{ place: 1, name: 'alder440' }], [{ place: 1, name: 'birch550' }], 'recorded', [['alder440'], [''], ['']], 'admin'],
    ['else the suggestion', [], [{ place: 1, name: 'birch550' }, { place: 2, name: 'cedar660' }], 'final_stage', [['birch550'], ['cedar660'], ['']], 'final_stage'],
    ['a tie, its players sharing their place', [], [{ place: 1, name: 'birch550' }, { place: 1, name: 'cedar660' }, { place: 3, name: 'elm770' }], 'final_stage', [['birch550', 'cedar660'], [''], ['elm770']], 'final_stage'],
    ['else nothing', [], [], '', [[''], [''], ['']], 'none'],
  ] as const)('starts from %s', (_, saved, suggested, from, names, origin) => {
    expect(podiumStart([...saved], [...suggested], from)).toEqual({ names, origin });
  });
});

describe('sharePlace', () => {
  it('adds an empty box to the place, leaving the others', () => {
    expect(sharePlace([['alder440'], ['birch550'], ['']], 1)).toEqual([['alder440'], ['birch550', ''], ['']]);
  });
});

describe('podiumNoteStart', () => {
  it.each([
    ['a saved note, whatever the origin', 'Organisers ruled on a tie', 'recorded', 'Organisers ruled on a tie'],
    ['nothing for a podium set by hand with no note', '', 'admin', ''],
    ['nothing when nothing was recorded or suggested', '', 'none', ''],
  ] as const)('starts the note from %s', (_, saved, origin, want) => {
    expect(podiumNoteStart(saved, origin)).toBe(want);
  });

  it.each(['recorded', 'final_stage'] as const)('says where a %s suggestion came from', (origin) => {
    expect(podiumNoteStart('', origin)).not.toBe('');
  });
});
