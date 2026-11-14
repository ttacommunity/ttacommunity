import { describe, expect, it } from 'vitest';
import { buildNewChannelBody } from './admin-movies';

describe('buildNewChannelBody', () => {
  it('requires channel and name', () => {
    expect(buildNewChannelBody('  ', 'Alice', '', false)).toEqual({
      ok: false,
      message: 'Enter a YouTube channel URL, @handle, or UC... ID.',
    });
    expect(buildNewChannelBody('https://youtube.com/@alice', '  ', '', false)).toEqual({
      ok: false,
      message: 'Enter a creator display name.',
    });
  });

  it('builds a trimmed payload with optional player', () => {
    expect(buildNewChannelBody(' @alice ', ' Alice Casts ', ' Alice ', true)).toEqual({
      ok: true,
      body: { channel: '@alice', name: 'Alice Casts', player: 'Alice', include_all: true },
    });
    expect(buildNewChannelBody('UC123', 'Bob Plays', '  ', false)).toEqual({
      ok: true,
      body: { channel: 'UC123', name: 'Bob Plays', include_all: false },
    });
  });
});
