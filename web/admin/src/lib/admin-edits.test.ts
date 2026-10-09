import { describe, expect, it } from 'vitest';
import { badgeChanged, canAddStory, cardChanged, isDiscordId, movedStory, regulationsSavable, storiesChanged, textChanged } from './admin-edits';

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
});
