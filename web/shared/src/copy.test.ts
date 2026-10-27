import { describe, expect, it } from 'vitest';
import { copyText } from './copy';

describe('copyText', () => {
  it('writes the text to the clipboard and says it did', async () => {
    const written: string[] = [];
    const clipboard = { writeText: async (text: string) => void written.push(text) };
    expect(await copyText(clipboard, 'ABCD1234')).toBe(true);
    expect(written).toEqual(['ABCD1234']);
  });

  it.each([
    ['refuses', { writeText: () => Promise.reject(new Error('not allowed')) }],
    ['is missing, as on a page not served over HTTPS', undefined],
  ])('says it could not copy when the clipboard %s', async (_, clipboard) => {
    expect(await copyText(clipboard, 'ABCD1234')).toBe(false);
  });
});
