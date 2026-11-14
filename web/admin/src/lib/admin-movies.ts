import type { components } from '@ttac/shared/api';

export type AdminMoviesState = components['schemas']['AdminMoviesState'];
export type AdminVideoChannel = components['schemas']['AdminVideoChannel'];
export type AdminVideo = components['schemas']['AdminVideo'];
export type MoviesSyncResult = components['schemas']['MoviesSyncResult'];

/** Validates and normalizes the Add Creator form fields before sending to the API. */
export function buildNewChannelBody(channel: string, name: string, player: string, includeAll: boolean): {
  ok: true;
  body: components['schemas']['NewVideoChannel'];
} | {
  ok: false;
  message: string;
} {
  const c = channel.trim();
  const n = name.trim();
  const p = player.trim();
  if (!c) return { ok: false, message: 'Enter a YouTube channel URL, @handle, or UC... ID.' };
  if (!n) return { ok: false, message: 'Enter a creator display name.' };
  return {
    ok: true,
    body: {
      channel: c,
      name: n,
      ...(p ? { player: p } : {}),
      include_all: includeAll,
    },
  };
}
