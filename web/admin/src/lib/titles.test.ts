import { describe, expect, it } from 'vitest';
import { titleName } from '@ttac/shared/ratings';
import { ADMIN_TITLES, titleLabel, type AdminTitle } from './titles';

describe('titleLabel', () => {
  it.each(ADMIN_TITLES.filter((t): t is Exclude<AdminTitle, ''> => t !== ''))('names %s as the site does', (t) => {
    expect(titleLabel(t)).toContain(titleName(t));
  });

  it('labels taking a title away apart from every title', () => {
    const named = ADMIN_TITLES.filter((t) => t !== '').map(titleLabel);
    expect(named).not.toContain(titleLabel(''));
  });
});
