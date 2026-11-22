import { describe, expect, it } from 'vitest';
import { buttonClass, groupClass } from './segmented-look';

describe('groupClass', () => {
  it.each([
    { name: 'a choice is the format bar', look: 'choice' as const, stacked: false, want: 'format-bar-group' },
    { name: 'tabs are the records tabs', look: 'tabs' as const, stacked: false, want: 'ach-tabs' },
    { name: 'a filter is the year pills', look: 'filter' as const, stacked: false, want: 'year-pills' },
    { name: 'a stacked choice puts one option on each line', look: 'choice' as const, stacked: true, want: 'format-bar-group stacked' },
  ])('$name', ({ look, stacked, want }) => {
    expect(groupClass(look, stacked)).toBe(want);
  });
});

describe('buttonClass', () => {
  it.each([
    { name: 'a choice button', look: 'choice' as const, chosen: false, want: 'format-btn' },
    { name: 'the chosen tab is pressed', look: 'tabs' as const, chosen: true, want: 'records-tab-btn active' },
    { name: 'a filter pill', look: 'filter' as const, chosen: false, want: 'year-pill' },
  ])('$name', ({ look, chosen, want }) => {
    expect(buttonClass(look, chosen)).toBe(want);
  });
});
