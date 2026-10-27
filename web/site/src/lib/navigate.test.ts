import { describe, expect, it, vi } from 'vitest';
import { followsLink, viewLinks } from './navigate';

const click = (change: Partial<{ metaKey: boolean; ctrlKey: boolean; shiftKey: boolean; button: number }> = {}) => ({
  metaKey: false,
  ctrlKey: false,
  shiftKey: false,
  button: 0,
  preventDefault: vi.fn(),
  ...change,
});

describe('followsLink', () => {
  it.each([
    ['a plain click', click(), false],
    ['Cmd-click', click({ metaKey: true }), true],
    ['Ctrl-click', click({ ctrlKey: true }), true],
    ['Shift-click', click({ shiftKey: true }), true],
    ['a middle click', click({ button: 1 }), true],
  ])('says whether the browser opens %s itself', (_, e, want) => {
    expect(followsLink(e)).toBe(want);
  });
});

describe('viewLinks', () => {
  const params = (page: number) => new URLSearchParams({ page: String(page) });

  it('points the link at the view', () => {
    expect(viewLinks(params, () => {})(3).href).toBe('?page=3');
  });

  it('goes to the view on a plain click, without the browser loading it', () => {
    const go = vi.fn();
    const e = click();
    viewLinks(params, go)(3).onClick(e);
    expect(go).toHaveBeenCalledWith(3);
    expect(e.preventDefault).toHaveBeenCalled();
  });

  it('leaves a Ctrl-click to the browser', () => {
    const go = vi.fn();
    const e = click({ ctrlKey: true });
    viewLinks(params, go)(3).onClick(e);
    expect(go).not.toHaveBeenCalled();
    expect(e.preventDefault).not.toHaveBeenCalled();
  });
});
