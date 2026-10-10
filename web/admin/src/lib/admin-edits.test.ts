import { describe, expect, it } from 'vitest';
import {
  addedStory,
  badgeChanged,
  canAddStory,
  cardChanged,
  editedStory,
  isDiscordId,
  isRemoval,
  movedStory,
  regulationsSavable,
  regulationsSource,
  regulationsStart,
  removedStory,
  storiesChanged,
  textChanged,
} from './admin-edits';

const CARD = { icon: '🏆', tagline: 'Annual', summary: '', subtitle: '', cadence: '' };
const STORY = (title: string) => ({ player: 'ann', badge: '', title, text: '' });

describe('admin edits', () => {
  it.each([
    ['a Discord id', ' 100000000000000001 ', true],
    ['too short', '1234', false],
    ['letters', '10000000000000000a', false],
    ['empty', '', false],
  ])('knows %s', (_, text, want) => {
    expect(isDiscordId(text)).toBe(want);
  });

  it.each([
    ['the same, but for spaces', 'Best of three', ' Best of three ', false],
    ['different', 'Best of three', 'Best of five', true],
    ['emptied', 'Best of three', '  ', true],
  ])('sees a note as changed when %s', (_, saved, text, want) => {
    expect(textChanged(saved, text)).toBe(want);
  });

  it.each([
    ['unchanged', CARD, false],
    ['one field changed', { ...CARD, cadence: 'Every year' }, true],
  ])('sees a card as changed when %s', (_, card, want) => {
    expect(cardChanged(CARD, card)).toBe(want);
  });

  it.each([
    ['new text', 'Best of three.', 'Ours.', 'CGE’s.', true],
    ['the text shown already', ' Ours. ', 'Ours.', 'CGE’s.', false],
    ['CGE’s text while none of ours is set', 'CGE’s.', '', 'CGE’s.', false],
    ['nothing', '  ', 'Ours.', 'CGE’s.', false],
  ])('lets regulations be saved with %s', (_, text, ours, cge, want) => {
    expect(regulationsSavable(text, ours, cge)).toBe(want);
  });

  it.each([
    ['the same', [STORY('A'), STORY('B')], false],
    ['reordered', [STORY('B'), STORY('A')], true],
    ['one removed', [STORY('B')], true],
  ])('sees stories as changed when %s', (_, list, want) => {
    expect(storiesChanged([STORY('A'), STORY('B')], list)).toBe(want);
  });

  it.each([
    ['up', 1, -1, ['B', 'A', 'C']],
    ['down', 1, 1, ['A', 'C', 'B']],
    ['past the top, nowhere', 0, -1, ['A', 'B', 'C']],
  ])('moves a story %s', (_, i, by, want) => {
    expect(movedStory([STORY('A'), STORY('B'), STORY('C')], i, by).map((s) => s.title)).toEqual(want);
  });

  it.each([
    [11, true],
    [12, false],
  ])('lets a story be added to %i', (n, want) => {
    expect(canAddStory(Array.from({ length: n }, (_, i) => STORY(String(i))))).toBe(want);
  });

  it.each([
    ['unchanged but for spaces', ' Founder ', 'Was there', false],
    ['a new description', 'Founder', 'Was here', true],
    ['no label', '  ', 'Was here', false],
  ])('lets a badge be saved when %s', (_, label, description, want) => {
    expect(badgeChanged({ label: 'Founder', description: 'Was there' }, label, description)).toBe(want);
  });

  it.each([
    ['nothing', '  ', true],
    ['text', ' Best of three ', false],
  ])('sees saving %s as a removal', (_, text, want) => {
    expect(isRemoval(text)).toBe(want);
  });

  it.each([
    ['ours, over CGE’s', 'Ours.', 'CGE’s.', 'Ours.', 'ours'],
    ['CGE’s, when none of ours is set', '', 'CGE’s.', 'CGE’s.', 'cge'],
    ['nothing, when neither is set', '', '', '', 'none'],
  ] as const)('starts regulations from %s', (_, ours, cge, text, source) => {
    expect(regulationsStart(ours, cge)).toBe(text);
    expect(regulationsSource(ours, cge)).toBe(source);
  });

  it('edits one field of one story', () => {
    const got = editedStory([STORY('A'), STORY('B')], 1, 'text', 'Won it all');
    expect(got.map((s) => [s.title, s.text])).toEqual([['A', ''], ['B', 'Won it all']]);
  });

  it('removes one story, keeping the order of the rest', () => {
    expect(removedStory([STORY('A'), STORY('B'), STORY('C')], 1).map((s) => s.title)).toEqual(['A', 'C']);
  });

  it('adds an empty story at the end', () => {
    expect(addedStory([STORY('A')])).toEqual([STORY('A'), { player: '', badge: '', title: '', text: '' }]);
  });
});
