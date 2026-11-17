/** The Discord bot tab's decisions: what to say about the commands' registration, a command's settings in a line, the choices of each setting, and whether an edit can be saved. */
import type { components } from '@ttac/shared/api';

type Bot = components['schemas']['DiscordBot'];
type Command = components['schemas']['DiscordBotCommand'];
type Reply = components['schemas']['DiscordReplyVisibility'];

/** STATES are a command's states, as its row's select offers them. */
export const STATES: readonly { value: 'on' | 'off'; label: string }[] = [
  { value: 'on', label: 'On' },
  { value: 'off', label: 'Off' },
];

/** REPLIES are who may see a command's reply. */
export const REPLIES: readonly { value: Reply; label: string }[] = [
  { value: 'private', label: 'Private unless shared' },
  { value: 'shared', label: 'Shared unless private' },
  { value: 'always_private', label: 'Always private' },
  { value: 'always_shared', label: 'Always shared' },
];

/** FORMATS are the formats a command may show when its asker gives none. */
export const FORMATS: readonly { value: string; label: string }[] = [
  { value: '0', label: 'Every format' },
  { value: '2', label: '2-player' },
  { value: '3', label: '3-player' },
  { value: '4', label: '4-player' },
];

/** ROWS are how many rows a list may show. */
export const ROWS: readonly { value: string; label: string }[] = Array.from({ length: 10 }, (_, i) => ({ value: String(i + 1), label: `${i + 1} rows` }));

/** registration says when the commands were last registered with Discord, its time written with when, and names any command Discord does not hold yet. */
export function registration(bot: Pick<Bot, 'commands' | 'registered_at' | 'registered'>, when: (iso: string) => string): string {
  if (!bot.registered_at) return 'Not registered with Discord yet: the api registers the commands as it starts, when the bot is set up.';
  const missing = bot.commands.map((c) => c.name).filter((n) => !bot.registered.includes(n));
  const said = `Registered with Discord ${when(bot.registered_at)}: ${bot.registered.length} commands.`;
  return missing.length === 0 ? said : `${said} Not there yet: ${missing.map((n) => `/${n}`).join(', ')} (the next api start registers them).`;
}

/** commandSettings says a command's settings in a line: who sees its reply, its format when it has one, its rows when it lists. */
export function commandSettings(c: Pick<Command, 'reply' | 'format' | 'results'>): string {
  const parts = [REPLIES.find((r) => r.value === c.reply)?.label ?? c.reply];
  if (c.format !== undefined && c.format !== null) parts.push(FORMATS.find((f) => f.value === String(c.format))?.label ?? `${c.format}-player`);
  if (c.results !== undefined && c.results !== null) parts.push(`${c.results} rows`);
  return parts.join(' · ');
}

/** commandChanged says whether a command's settings as edited differ from its saved ones. */
export function commandChanged(saved: Pick<Command, 'reply' | 'format' | 'results'>, draft: Pick<Command, 'reply' | 'format' | 'results'>): boolean {
  return saved.reply !== draft.reply || (saved.format ?? null) !== (draft.format ?? null) || (saved.results ?? null) !== (draft.results ?? null);
}
