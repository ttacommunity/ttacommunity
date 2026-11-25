/** The Discord bot, for admins: whether its commands are registered with Discord, the link that adds it to a server, and each command, on or off, with its settings to edit. */
import { useEffect, useState } from 'react';
import type { components } from '@ttac/shared/api';
import type { Load } from '@ttac/shared/load';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import * as req from '../lib/admin-requests';
import { STATES, commandSettings, registration } from '../lib/discord-bot';
import { outcomeOf, shownAt, succeeded, type Outcome } from '../lib/outcome';
import Notice, { Loading } from './Notice';
import AdminCard from './AdminCard';
import AdminTable from './AdminTable';
import CommandSheet from './CommandSheet';
import RowSelect from './RowSelect';

type Bot = components['schemas']['DiscordBot'];
type Command = components['schemas']['DiscordBotCommand'];

export default function DiscordBotPanel() {
  const [bot, setBot] = useState<Load<Bot>>({ kind: 'loading' });
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [editing, setEditing] = useState<{ command: Command; opener: HTMLElement } | null>(null);
  const zone = viewerZone();
  const when = (iso: string) => formatInstant(iso, zone);
  const load = async () => {
    const r = await req.send<Bot>(req.discordBot);
    setBot(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    void load();
  }, []);
  const turn = async (c: Command, on: boolean) => {
    const r = await req.send(req.setCommand(c.name, { on, reply: c.reply, format: c.format ?? undefined, results: c.results ?? undefined }));
    setOutcome(outcomeOf(r, on ? `Turned /${c.name} on.` : `Turned /${c.name} off: it now answers that it is off.`, 'bot'));
    await load();
  };
  return (
    <AdminCard id="discord-bot-heading" title="Discord bot">
      <Loading load={bot} what="the bot" />
      {bot.kind === 'ready' && (
        <>
          <p className="muted">
            {registration(bot.data, when)} Who may use each command, and where, is set in Discord: Server Settings, Integrations.
          </p>
          <p>
            {bot.data.install_url ? (
              <a href={bot.data.install_url} target="_blank" rel="noopener noreferrer">
                Add the bot's commands to a server
              </a>
            ) : (
              <span className="muted">No Discord application is set up here, so there is no link to add the bot.</span>
            )}
          </p>
          <Notice outcome={shownAt(outcome, 'bot')} onClose={() => setOutcome(null)} />
          <AdminTable caption="The bot's commands, each on or off, with its settings">
            <thead>
              <tr>
                <th>Command</th>
                <th>What it does</th>
                <th>State</th>
                <th>Settings</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {bot.data.commands.map((c) => (
                <tr key={c.name}>
                  <td>/{c.name}</td>
                  <td>{c.description}</td>
                  <td>
                    <RowSelect label={`Whether /${c.name} is on`} value={c.on ? 'on' : 'off'} options={STATES} onChange={(v) => void turn(c, v === 'on')} />
                  </td>
                  <td>{commandSettings(c)}</td>
                  <td>
                    <button type="button" className="btn-antique btn-compact" onClick={(e) => setEditing({ command: c, opener: e.currentTarget })}>
                      Edit
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
          {editing !== null && (
            <CommandSheet
              command={editing.command}
              opener={editing.opener}
              onSaved={() => {
                setOutcome(succeeded(`Saved /${editing.command.name}.`, 'bot'));
                void load();
              }}
              onClose={() => setEditing(null)}
            />
          )}
        </>
      )}
    </AdminCard>
  );
}
