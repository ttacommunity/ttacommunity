/** Notifications, for admins: the Discord webhooks the api posts link requests and blitz reports to, each tested here. */
import { useEffect, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import { useDialog } from '@ttac/shared/Dialog';
import * as req from '../lib/admin-requests';
import { adminRequest } from '../lib/admin-api';
import { EVENTS, endpointChanged, toggled } from '../lib/notification-endpoints';
import { loadFailure, outcomeOf, refused, shownAt, succeeded, type Outcome } from '../lib/outcome';
import Notice from './Notice';

type Endpoint = components['schemas']['NotificationEndpoint'];
type NotificationTest = components['schemas']['NotificationTest'];
type Draft = Pick<Endpoint, 'name' | 'url' | 'events' | 'prefix'>;

const EMPTY: Draft = { name: '', url: '', events: [], prefix: '' };

/** EndpointFields edits an endpoint's name, webhook URL, prefix and events; id keeps each field's id its own. */
function EndpointFields({ id, draft, onChange }: { id: string; draft: Draft; onChange: (d: Draft) => void }) {
  return (
    <>
      <div className="admin-add">
        <label>
          Name
          <input className="input-antique" id={`${id}-name`} value={draft.name} maxLength={100} onChange={(e) => onChange({ ...draft, name: e.target.value })} required />
        </label>
        <label className="title-reason">
          Discord webhook URL
          <input className="input-antique" id={`${id}-url`} type="url" value={draft.url} placeholder="https://discord.com/api/webhooks/…"
            onChange={(e) => onChange({ ...draft, url: e.target.value })} required />
        </label>
        <label>
          Prefix (such as [dev])
          <input className="input-antique" id={`${id}-prefix`} value={draft.prefix} maxLength={20} onChange={(e) => onChange({ ...draft, prefix: e.target.value })} />
        </label>
      </div>
      <fieldset className="check-kinds">
        <legend>Posts</legend>
        {EVENTS.map(({ key, label }) => (
          <label key={key}>
            <input type="checkbox" checked={draft.events.includes(key)} onChange={(e) => onChange({ ...draft, events: toggled(draft.events, key, e.target.checked) })} /> {label}
          </label>
        ))}
      </fieldset>
    </>
  );
}

/** EndpointRow is one endpoint to change, test or remove, with what became of the last action on it. */
function EndpointRow({ endpoint, onReload, outcome, say }: { endpoint: Endpoint; onReload: () => Promise<void>; outcome: Outcome | null; say: (o: Outcome | null) => void }) {
  const at = `endpoint ${endpoint.id}`;
  const [draft, setDraft] = useState<Draft>(endpoint);
  const dialog = useDialog();
  const save = async () => {
    const r = await req.send(req.setEndpoint(endpoint.id, draft));
    say(outcomeOf(r, `Saved ${draft.name.trim()}.`, at));
    await onReload();
  };
  const test = async () => {
    const r = await req.send<NotificationTest>(req.testEndpoint(endpoint.id));
    if (!r.ok) say(refused(r.message, at));
    else say(r.data.delivered ? succeeded(r.data.message, at) : refused(r.data.message, at));
  };
  const remove = async (opener: HTMLElement) => {
    const sure = await dialog.confirm({ heading: `Remove ${endpoint.name}?`, text: 'Nothing more is posted to it.', action: 'Remove', final: true }, opener);
    if (!sure) return;
    const r = await req.send(req.removeEndpoint(endpoint.id));
    say(outcomeOf(r, `Removed ${endpoint.name}.`, at, 'endpoints'));
    await onReload();
  };
  return (
    <div className="badge-row">
      {dialog.element}
      <EndpointFields id={`endpoint-${endpoint.id}`} draft={draft} onChange={setDraft} />
      <div className="admin-add">
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={!endpointChanged(endpoint, draft)}>
          Save
        </button>
        <button type="button" className="btn-antique" onClick={() => void test()}>
          Test
        </button>
        <button type="button" className="btn-antique" onClick={(e) => void remove(e.currentTarget)}>
          Remove
        </button>
      </div>
      <Notice outcome={shownAt(outcome, at)} onClose={() => say(null)} />
    </div>
  );
}

/** NotificationsPanel lists the endpoints and adds new ones. */
export default function NotificationsPanel() {
  const [list, setList] = useState<Endpoint[] | null>(null);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [draft, setDraft] = useState<Draft>(EMPTY);

  const load = async () => {
    const r = await adminRequest<Endpoint[]>('/api/notification-endpoints', {});
    if (r.ok) setList(r.data);
    else setOutcome(loadFailure(r.message, 'endpoints'));
  };
  useEffect(() => {
    void load();
  }, []);

  const add = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send<Endpoint>(req.addEndpoint(draft));
    setOutcome(r.ok ? succeeded(`Added ${r.data.name}. Test it to see a message arrive.`, 'add endpoint') : refused(r.message, 'add endpoint'));
    if (r.ok) setDraft(EMPTY);
    await load();
  };

  return (
    <section className="rating-card admin-card" aria-labelledby="notifications-heading">
      <h2 id="notifications-heading" className="rating-card-heading">
        Notifications
      </h2>
      <p className="muted">
        Discord channels the site posts to when a new player signs up, a player asks to be linked, or one reports, corrects or flags a blitz result.
        Make a webhook in the channel's settings (Integrations, Webhooks) and paste its URL here.
      </p>
      <Notice outcome={shownAt(outcome, 'endpoints')} onClose={() => setOutcome(null)} />
      {list?.map((e) => (
        <EndpointRow key={`${e.id}-${e.name}-${e.url}-${e.prefix}-${e.events.join()}`} endpoint={e} onReload={load} outcome={outcome} say={setOutcome} />
      ))}
      {list !== null && list.length === 0 && <p className="muted">No endpoints yet: nothing is posted.</p>}
      <form onSubmit={(e) => void add(e)}>
        <h3>Add an endpoint</h3>
        <EndpointFields id="endpoint-new" draft={draft} onChange={setDraft} />
        <button type="submit" className="btn-antique">
          Add endpoint
        </button>
      </form>
      <Notice outcome={shownAt(outcome, 'add endpoint')} onClose={() => setOutcome(null)} />
    </section>
  );
}
