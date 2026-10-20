/** The account page: how to sign in, with Discord or an emailed link; once signed in, the ways in (added or removed), asking to be linked to a TTA username, or, linked, the player's own country and zone and any ban; signing out, and deleting the account. A first sign-in lands here, with the way back to the page it started from. */
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import { currentRun, leaderboard } from '@ttac/shared/ratings';
import {
  accountState,
  addDiscordHref,
  addEmail,
  askToBeLinked,
  backLabel,
  backPath,
  banNotice,
  browserStorage,
  changeMyDetails,
  countryOptions,
  deleteAccount,
  deleteQuestion,
  draftChanges,
  fetchCountries,
  fetchMe,
  isFirstSignIn,
  rememberSignedIn,
  removeWayIn,
  saveOutcome,
  sharedMe,
  signedInAs,
  signInHref,
  signInNotice,
  signOut,
  suggestedCountry,
  waysIn,
  withoutSignInParam,
  zoneOptions,
  type AccountState,
  type Country,
  type Done,
  type Draft,
  type Me,
  type SaveOutcome,
} from '../lib/account';
import { sendEmailLink } from '../lib/email-sign-in';
import { DEFAULT_VIEW, modelOf } from '../lib/leaderboard-view';
import SearchBox, { type SearchPlayer } from './SearchBox';
import Turnstile from './Turnstile';
import Waiting from './Waiting';
import './Account.css';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; me: Me | null };

type Props = { apiUrl: string; adminUrl: string; turnstileSitekey: string };

/** AccountPage asks who is signed in and shows what they can do next. */
export default function AccountPage({ apiUrl, adminUrl, turnstileSitekey }: Props) {
  const [params] = useState(() => new URLSearchParams(window.location.search));
  const notice = signInNotice(params);
  const first = isFirstSignIn(params);
  const back = backPath(params.get('back'));
  const [load, setLoad] = useState<Load>({ kind: 'loading' });

  useEffect(() => {
    if (params.has('signin')) window.history.replaceState(null, '', withoutSignInParam(window.location.pathname, window.location.search));
    sharedMe(apiUrl)
      .then((me) => {
        rememberSignedIn(browserStorage(), me !== null);
        setLoad({ kind: 'ready', me });
      })
      .catch((e: unknown) => setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
  }, [apiUrl, params]);

  // refresh reads the account again after a change.
  const refresh = () =>
    fetchMe(apiUrl)
      .then((me) => setLoad({ kind: 'ready', me }))
      .catch((e: unknown) => setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));

  return (
    <>
      <div className="tournaments-header">
        <h1 className="tournaments-title">Your account</h1>
        <div className="tournaments-subtitle">Sign in to take part in what the community runs, as the player you are on CGE.</div>
      </div>
      {notice !== null && (
        <p className={`rating-card account-card account-notice account-notice-${notice.kind}`} role="alert">
          {notice.text}
        </p>
      )}
      {load.kind !== 'ready' ? (
        <Waiting load={load} what="your account" />
      ) : (
        <Account
          apiUrl={apiUrl}
          adminUrl={adminUrl}
          turnstileSitekey={turnstileSitekey}
          me={load.me}
          state={accountState(load.me)}
          first={first}
          back={back}
          refresh={refresh}
        />
      )}
    </>
  );
}

type AccountProps = Props & { me: Me | null; state: AccountState; first: boolean; back: string | null; refresh: () => void };

function Account({ apiUrl, adminUrl, turnstileSitekey, me, state, first, back, refresh }: AccountProps) {
  if (me === null || state.kind === 'signed-out') return <SignIn apiUrl={apiUrl} turnstileSitekey={turnstileSitekey} back={back} />;
  const who = signedInAs(me);
  return (
    <section className="rating-card account-card" aria-labelledby="account-who">
      <h2 id="account-who" className="rating-card-heading">
        {first ? `Welcome, ${me.display_name || me.username}` : who.heading}
      </h2>
      {who.detail !== '' && <p className="account-muted">{who.detail}</p>}
      {first && <p>You are signed in. Add your details now, or go back and add them any time from this page.</p>}
      {state.kind === 'linked' ? <Linked adminUrl={adminUrl} state={state} /> : <LinkStatus state={state} />}
      <Details apiUrl={apiUrl} me={me} state={state} back={back} refresh={refresh} />
      <WaysIn apiUrl={apiUrl} me={me} refresh={refresh} />
      <SignOut apiUrl={apiUrl} state={state} />
    </section>
  );
}

/** BackLink goes back to the page the player came from, named for whether leaving now would lose anything. */
function BackLink({ back, changed, saved }: { back: string | null; changed: boolean; saved: boolean }) {
  if (back === null) return null;
  return (
    <a className="btn-antique account-back" href={back}>
      {backLabel(changed, saved)}
    </a>
  );
}

function SignIn({ apiUrl, turnstileSitekey, back }: { apiUrl: string; turnstileSitekey: string; back: string | null }) {
  const returnTo = back ?? window.location.pathname;
  return (
    <section className="rating-card account-card" aria-labelledby="account-sign-in">
      <h2 id="account-sign-in" className="rating-card-heading">
        Sign in
      </h2>
      <ul className="account-ways">
        <li>
          <a className="btn-antique" href={signInHref(apiUrl, returnTo)}>
            Sign in with Discord
          </a>
          <p className="account-muted">Needs a Discord account. The TTA community, its events and its tournaments are organised on Discord, so we recommend it.</p>
        </li>
        <li>
          <EmailSignIn apiUrl={apiUrl} turnstileSitekey={turnstileSitekey} back={returnTo} />
        </li>
      </ul>
      <p className="account-muted">
        Discord tells us only your username and display name: never your email or your servers. By email, we keep your address, to send your links. Then
        you link your sign-in to your TTA username, and an admin or organiser checks it is you. <a href="/privacy/">What we keep, and how to have it removed</a>
        .
      </p>
    </section>
  );
}

/** EmailSignIn is the way in for players without Discord: a button that opens the form (an address and Turnstile's check, loaded only then), then a link to the inbox. */
function EmailSignIn({ apiUrl, turnstileSitekey, back }: { apiUrl: string; turnstileSitekey: string; back: string }) {
  const [open, setOpen] = useState(false);
  const [address, setAddress] = useState('');
  const [token, setToken] = useState('');
  const [round, setRound] = useState(0);
  const [sentTo, setSentTo] = useState('');
  const [problem, setProblem] = useState('');
  const [busy, setBusy] = useState(false);
  const send = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setProblem('');
    const done = await sendEmailLink(apiUrl, address, token, back);
    setBusy(false);
    setRound((r) => r + 1);
    if (done.ok) setSentTo(address.trim());
    else setProblem(done.message);
  };
  if (sentTo !== '') {
    return (
      <div role="status">
        <p>
          Check your inbox: a sign-in link is on its way to <strong>{sentTo}</strong>. It works once, within the hour.
        </p>
        <button type="button" className="btn-antique" onClick={() => setSentTo('')}>
          Send another
        </button>
      </div>
    );
  }
  return (
    <>
      <button type="button" className="btn-antique" aria-expanded={open} aria-controls="account-email-form" onClick={() => setOpen((o) => !o)}>
        Sign in with email
      </button>
      <p className="account-muted">For players without Discord: we email you a link that signs you in.</p>
      {open && <EmailForm address={address} setAddress={setAddress} token={token} setToken={setToken} round={round} busy={busy} problem={problem} turnstileSitekey={turnstileSitekey} send={send} />}
    </>
  );
}

type EmailFormProps = {
  address: string;
  setAddress: (a: string) => void;
  token: string;
  setToken: (t: string) => void;
  round: number;
  busy: boolean;
  problem: string;
  turnstileSitekey: string;
  send: (e: FormEvent) => Promise<void>;
};

/** EmailForm asks for the address, with Turnstile's check, once the player opens it. */
function EmailForm({ address, setAddress, token, setToken, round, busy, problem, turnstileSitekey, send }: EmailFormProps) {
  return (
    <form id="account-email-form" className="account-form" onSubmit={(e) => void send(e)}>
      <label htmlFor="account-email">Your email address</label>
      <input
        id="account-email"
        className="input-antique"
        type="email"
        autoComplete="email"
        required
        maxLength={254}
        value={address}
        onChange={(e) => setAddress(e.target.value)}
      />
      <Turnstile sitekey={turnstileSitekey} round={round} onToken={setToken} />
      <div className="account-row">
        <button type="submit" className="btn-antique" disabled={busy || token === '' || address.trim() === ''}>
          {busy ? 'Sending…' : 'Email me a sign-in link'}
        </button>
      </div>
      {problem !== '' && (
        <p className="account-problem" role="alert">
          {problem}
        </p>
      )}
    </form>
  );
}

/** supportedZones are the zones this browser knows, none where it cannot list them. */
function supportedZones(): string[] {
  return typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : [];
}

/** displayNames names countries in English, or null where the browser cannot. */
function displayNames(): Intl.DisplayNames | null {
  try {
    return new Intl.DisplayNames(['en'], { type: 'region' });
  } catch {
    return null;
  }
}

/** Linked says which member the player is linked to, with their player page and, for admins and organisers, the way to admin. */
function Linked({ adminUrl, state }: { adminUrl: string; state: Extract<AccountState, { kind: 'linked' }> }) {
  return (
    <>
      <p>
        Linked to <strong>{state.member.name}</strong>
        {state.playerHref !== null && (
          <>
            {' '}
            · <a href={state.playerHref}>your player page</a>
          </>
        )}
      </p>
      <Banned state={state} />
      {state.adminRole !== null && (
        <p>
          You are {state.adminRole === 'admin' ? 'an admin' : 'an organiser'}: <a href={adminUrl}>open admin</a>.
        </p>
      )}
    </>
  );
}

/** Banned tells a banned member they are kept out of the events we host, and why. */
function Banned({ state }: { state: AccountState }) {
  const ban = banNotice(state);
  if (ban === null) return null;
  return (
    <div className="account-ban">
      <strong className="account-ban-heading">Banned</strong>
      <span>You cannot be entered in the events we host. If you think that is a mistake, message the organisers.</span>
      {ban.reason !== '' && <span>Reason: {ban.reason}</span>}
    </div>
  );
}

/** LinkStatus says where a request to be linked stands. */
function LinkStatus({ state }: { state: Exclude<AccountState, { kind: 'linked' | 'signed-out' }> }) {
  if (state.kind === 'waiting') {
    return (
      <p>
        You asked to be linked to <strong>{state.name}</strong>. An admin or organiser will check it is you; this page shows their answer.
      </p>
    );
  }
  if (state.kind === 'refused') {
    return (
      <p>
        Your request to be linked to <strong>{state.name}</strong> was not approved{state.reason === '' ? '.' : `: ${state.reason}`} You may ask again.
      </p>
    );
  }
  return <p>Link your sign-in to your TTA username, so the site knows which player you are.</p>;
}

type DetailsProps = { apiUrl: string; me: Me; state: Exclude<AccountState, { kind: 'signed-out' }>; back: string | null; refresh: () => void };

/** Details is the player's form: the TTA username to be linked to while not linked, and their country and zone, saved together. */
function Details({ apiUrl, me, state, back, refresh }: DetailsProps) {
  const linked = state.kind === 'linked';
  const saved: Draft = { name: state.kind === 'waiting' || state.kind === 'refused' ? state.name : '', country: me.country ?? '', zone: me.zone };
  const [draft, setDraft] = useState<Draft>(saved);
  const [countries, setCountries] = useState<Country[]>([]);
  const [players, setPlayers] = useState<SearchPlayer[]>([]);
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const [busy, setBusy] = useState(false);
  const changes = draftChanges(saved, draft);
  const asking = !linked && changes.name;
  const changed = asking || changes.details;

  useEffect(() => {
    fetchCountries(apiUrl)
      .then((codes) => setCountries(countryOptions(codes, displayNames())))
      .catch(() => setCountries([]));
  }, [apiUrl]);

  // The ratings pages' search lists the rated players; only a player not linked yet types a name.
  useEffect(() => {
    if (linked) return;
    currentRun(apiUrl)
      .then((run) => leaderboard(apiUrl, run.run_id, modelOf(DEFAULT_VIEW), DEFAULT_VIEW.format))
      .then((board) => setPlayers(board.players))
      .catch(() => setPlayers([]));
  }, [apiUrl, linked]);

  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setOutcome(null);
    const asked = asking ? await askToBeLinked(apiUrl, draft.name) : null;
    const details = changes.details && asked?.ok !== false ? await changeMyDetails(apiUrl, { country: draft.country, zone: draft.zone }) : null;
    setBusy(false);
    setOutcome(saveOutcome(asked, details));
    if (asked?.ok === true || details?.ok === true) refresh();
  };

  const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const suggestion = suggestedCountry(players.map((p) => ({ name: p.name, country: p.country ?? null })), draft.name, draft.zone, browserZone);
  const suggestionName = countries.find((c) => c.code === suggestion)?.name ?? suggestion;
  return (
    <form className="account-form" onSubmit={(e) => void save(e)}>
      <h3 className="account-subheading">Your details</h3>
      {!linked && (
        <>
          <label htmlFor="account-tta-name">Your TTA username, as in the app</label>
          <SearchBox
            id="account-tta-name"
            players={players}
            value={draft.name}
            placeholder=""
            wide
            onChange={(name) => setDraft((d) => ({ ...d, name }))}
          />
        </>
      )}
      <label htmlFor="account-country">Country, for the flag beside your name</label>
      <div className="account-row">
        <select id="account-country" className="input-antique" value={draft.country} onChange={(e) => setDraft({ ...draft, country: e.target.value })}>
          <option value="">None</option>
          {countries.length === 0 && draft.country !== '' && <option value={draft.country}>{draft.country}</option>}
          {countries.map((c) => (
            <option key={c.code} value={c.code}>
              {c.name}
            </option>
          ))}
        </select>
        {suggestion !== null && suggestion !== draft.country && (
          <button type="button" className="btn-antique" onClick={() => setDraft((d) => ({ ...d, country: suggestion }))}>
            Use {suggestionName}
          </button>
        )}
      </div>
      <label htmlFor="account-zone">Time zone, for pairing and the times shown to you</label>
      <div className="account-row">
        <select id="account-zone" className="input-antique" value={draft.zone} onChange={(e) => setDraft({ ...draft, zone: e.target.value })}>
          {zoneOptions(supportedZones(), draft.zone).map((z) => (
            <option key={z} value={z}>
              {z}
            </option>
          ))}
        </select>
        {browserZone !== draft.zone && (
          <button type="button" className="btn-antique" onClick={() => setDraft({ ...draft, zone: browserZone })}>
            Use {browserZone}
          </button>
        )}
      </div>
      {!linked && <p className="account-muted">Your country and time zone also help an admin or organiser check it is you.</p>}
      <div className="account-row">
        <button type="submit" className="btn-antique" disabled={busy || !changed}>
          {busy ? 'Saving…' : 'Save'}
        </button>
        <BackLink back={back} changed={changed} saved={outcome?.kind === 'done' && !changed} />
      </div>
      <div role="status">
        <SaveNotice outcome={outcome} onClose={() => setOutcome(null)} />
      </div>
    </form>
  );
}

/** SaveNotice shows what a save did, as admin shows an action's outcome: Done or Not done on a panel bordered green or red, scrolled into view. */
function SaveNotice({ outcome, onClose }: { outcome: SaveOutcome | null; onClose: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (outcome !== null) box.current?.scrollIntoView({ block: 'nearest' });
  }, [outcome]);
  if (outcome === null) return null;
  return (
    <div ref={box} className={`account-outcome account-outcome-${outcome.kind}`}>
      <div>
        <strong className="account-outcome-heading">{outcome.heading}</strong>
        <span className="account-outcome-text">{outcome.text}</span>
      </div>
      <button type="button" className="account-outcome-close" aria-label="Close" onClick={onClose}>
        ×
      </button>
    </div>
  );
}

/** WaysIn lists the ways into the account, each removable while another remains, adds Discord or an email address, and changes the email address. */
function WaysIn({ apiUrl, me, refresh }: { apiUrl: string; me: Me; refresh: () => void }) {
  const { ways, canAdd } = waysIn(me);
  const dialog = useDialog();
  const [changing, setChanging] = useState(false);
  const [address, setAddress] = useState('');
  const [said, setSaid] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const remove = async (provider: (typeof ways)[number]['provider'], label: string) => {
    const sure = await dialog.confirm({ heading: `Remove ${label}?`, text: 'You will no longer sign in that way. You can add it again later.', action: 'Remove' });
    if (!sure) return;
    const done = await removeWayIn(apiUrl, provider);
    setSaid(done.ok ? { ok: true, text: `Removed ${label}.` } : { ok: false, text: done.message });
    if (done.ok) refresh();
  };
  const add = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    const done = await addEmail(apiUrl, address);
    setBusy(false);
    const sent = `Check your inbox: open the link sent to ${address.trim()} within the hour to ${changing ? 'change to it' : 'add it'}.`;
    setSaid(done.ok ? { ok: true, text: sent } : { ok: false, text: done.message });
    if (done.ok) {
      setAddress('');
      setChanging(false);
    }
  };
  return (
    <div className="account-form">
      {dialog.element}
      <h3 className="account-subheading">Ways you sign in</h3>
      <ul className="account-ways">
        {ways.map((w) => (
          <li key={w.provider} className="account-row">
            <span className="account-way">{w.label}</span>
            {w.changeable && !changing && (
              <button type="button" className="btn-antique" onClick={() => setChanging(true)}>
                Change
              </button>
            )}
            {w.removable && (
              <button type="button" className="btn-antique" onClick={() => void remove(w.provider, w.label)}>
                Remove
              </button>
            )}
          </li>
        ))}
      </ul>
      {canAdd.discord && (
        <div className="account-row">
          <a className="btn-antique" href={addDiscordHref(apiUrl)}>
            Add Discord
          </a>
        </div>
      )}
      {(canAdd.email || changing) && (
        <form className="account-form" onSubmit={(e) => void add(e)}>
          {changing ? (
            <label htmlFor="account-add-email">Your new email address. We email it a link: opening it changes your address, and the old one stops signing you in.</label>
          ) : (
            <>
              <label htmlFor="account-add-email">Add an email address to enable signing in using email.</label>
              <p className="account-muted">It also enables Admins and Organisers to contact you for events you sign up for if you don't have Discord.</p>
            </>
          )}
          <div className="account-row">
            <input
              id="account-add-email"
              className="input-antique"
              type="email"
              autoComplete="email"
              required
              maxLength={254}
              value={address}
              onChange={(e) => setAddress(e.target.value)}
            />
            <button type="submit" className="btn-antique" disabled={busy || address.trim() === ''}>
              {busy ? 'Sending…' : changing ? 'Email me a link to change it' : 'Email me a link to add it'}
            </button>
            {changing && (
              <button type="button" className="btn-antique" onClick={() => setChanging(false)}>
                Cancel
              </button>
            )}
          </div>
        </form>
      )}
      {said !== null && (
        <p className={said.ok ? undefined : 'account-problem'} role={said.ok ? 'status' : 'alert'}>
          {said.text}
        </p>
      )}
    </div>
  );
}

/** SignOut signs this browser out, or, once confirmed, deletes the account and signs out everywhere. */
function SignOut({ apiUrl, state }: { apiUrl: string; state: AccountState }) {
  const [problem, setProblem] = useState('');
  const dialog = useDialog();
  const leave = async (end: () => Promise<Done>, to: string) => {
    const done = await end();
    if (!done.ok) {
      setProblem(done.message);
      return;
    }
    rememberSignedIn(browserStorage(), false);
    window.location.assign(to);
  };
  const remove = async () => {
    if (await dialog.confirm(deleteQuestion(state))) await leave(() => deleteAccount(apiUrl), '/account/?signin=deleted');
  };
  return (
    <div className="account-sign-out account-row">
      {dialog.element}
      <button type="button" className="btn-antique" onClick={() => void leave(() => signOut(apiUrl), '/account/')}>
        Sign out
      </button>
      <button type="button" className="btn-antique btn-danger" onClick={() => void remove()}>
        Delete account
      </button>
      {problem !== '' && (
        <p className="account-problem" role="alert">
          {problem}
        </p>
      )}
    </div>
  );
}
