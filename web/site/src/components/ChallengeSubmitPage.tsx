/** Submitting dominations: for a signed-in player linked to a TTA username, a row per challenge, a review, then saving. */
import { useEffect, useState } from 'react';
import Segmented from '@ttac/shared/Segmented';
import { challengeEntries, challengeHighscores, type ChallengeEntries, type ChallengeHighscores } from '@ttac/shared/ratings';
import { change, sharedMe, signInHref, type Me } from '../lib/account';
import type { ChallengeTile } from '../lib/challenge-list';
import { bestsOf, proofOwners, reviewNotes, submission, type Row } from '../lib/challenge-submit';
import { challengePicture, challengePlayerHref, challengeSets, setLabel, type SetId } from '../lib/challenges';
import { ProofLink } from './ChallengeParts';
import { useLoad } from './useLoad';
import Waiting from './Waiting';

const SETS: SetId[] = ['original', 'expansion', 'mix'];

/** ChallengeSubmitPage asks who is signed in, then shows what they may do: sign in, get linked, or submit. */
export default function ChallengeSubmitPage({ apiUrl, list }: { apiUrl: string; list: ChallengeTile[] }) {
  const me = useLoad('me', () => sharedMe(apiUrl));
  return (
    <>
      <Examples />
      {me.kind !== 'ready' ? <Waiting load={me} what="your account" /> : <Gate apiUrl={apiUrl} list={list} me={me.data} />}
    </>
  );
}

function Gate({ apiUrl, list, me }: { apiUrl: string; list: ChallengeTile[]; me: Me | null }) {
  if (me === null) {
    return (
      <div className="rating-card challenges-card">
        <p>Sign in to submit your dominations: they are saved under your TTA username.</p>
        <a className="btn-antique" href={signInHref(apiUrl, 'discord', '/challenges/submit/')}>
          Sign in with Discord
        </a>
      </div>
    );
  }
  if (me.member === null) {
    return (
      <div className="rating-card challenges-card">
        <p>Your account is not linked to a TTA username yet. Ask to be linked on your account page; once an organiser approves it, you can submit here.</p>
        <a className="btn-antique" href="/account/">
          Your account
        </a>
      </div>
    );
  }
  if (me.member.banned) {
    return <div className="rating-card waiting-message">Your account may not submit dominations.</div>;
  }
  return <Form apiUrl={apiUrl} list={list} name={me.member.name} />;
}

function Form({ apiUrl, list, name }: { apiUrl: string; list: ChallengeTile[]; name: string }) {
  const [round, setRound] = useState(0);
  const entries = useLoad(`entries-${round}`, () => challengeEntries(apiUrl));
  const records = useLoad(`records-${round}`, () => challengeHighscores(apiUrl, true));
  const [set, setSet] = useState<SetId>('mix');
  const [rows, setRows] = useState<Record<string, Row>>({});
  const [shared, setShared] = useState('');
  const [reviewing, setReviewing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<{ ok: boolean; text: string } | null>(null);
  useEffect(() => setStatus(null), [rows, shared]);

  if (entries.kind !== 'ready') return <Waiting load={entries} what="the entries" />;
  if (records.kind !== 'ready') return <Waiting load={records} what="the records" />;
  const mine = bestsOf(entries.data.entries, name);
  const { entries: chosen, errors } = submission(list, rows, shared);
  const setRow = (id: string, patch: Partial<Row>) => setRows((r) => ({ ...r, [id]: { ...(r[id] ?? { domination: '', proof: '' }), ...patch } }));
  const filledIn = (s: SetId) => chosen.filter((e) => list.find((c) => c.id === e.challenge)?.set === s).length;

  const save = async () => {
    setBusy(true);
    const done = await change(fetch, apiUrl, 'POST', '/v1/me/challenges/entries', { entries: chosen });
    setBusy(false);
    if (!done.ok) {
      setStatus({ ok: false, text: done.message });
      return;
    }
    setStatus({ ok: true, text: `Saved ${chosen.length} ${chosen.length === 1 ? 'entry' : 'entries'}.` });
    setRows({});
    setShared('');
    setReviewing(false);
    setRound((n) => n + 1);
  };

  return (
    <>
      <div className="rating-card challenges-card">
        <p>
          Submitting as <strong>{name}</strong> · <a href={challengePlayerHref(name)}>your challenges page</a>
        </p>
        <label className="challenges-field">
          Discord link for every entry below without its own (optional, for verification)
          <input className="input-antique" type="url" placeholder="https://discord.com/channels/…" value={shared} onChange={(e) => setShared(e.target.value)} />
        </label>
      </div>

      {reviewing ? (
        <Review apiUrl={apiUrl} name={name} list={list} chosen={chosen} mine={mine} entries={entries.data} records={records.data} />
      ) : (
        <>
          <div className="challenges-bar">
            <Segmented
              label="Set"
              look="raised"
              value={set}
              onChange={setSet}
              options={SETS.map((s) => ({ value: s, label: filledIn(s) > 0 ? `${setLabel(s)} (${filledIn(s)})` : setLabel(s) }))}
            />
          </div>
          {challengeSets(list)
            .find((s) => s.id === set)
            ?.groups.map((g) => (
              <fieldset key={g.id} className="rating-card challenges-card challenges-submit-group">
                <legend className="rating-card-heading">{g.label}</legend>
                {g.challenges.map((c) => {
                  const row = rows[c.id] ?? { domination: '', proof: '' };
                  const best = mine.get(c.id);
                  return (
                    <div key={c.id} className="challenges-submit-row">
                      <img src={challengePicture(c.id)} alt="" width={56} height={47} loading="lazy" />
                      <div>
                        <div>{c.name}</div>
                        <div className="muted">
                          {c.players}P{best !== undefined && ` · your best ${best}`}
                        </div>
                      </div>
                      <input
                        className="input-antique"
                        type="number"
                        step={1}
                        placeholder="Domination"
                        aria-label={`${c.name} domination`}
                        value={row.domination}
                        onChange={(e) => setRow(c.id, { domination: e.target.value })}
                      />
                      <input
                        className="input-antique"
                        type="url"
                        placeholder={shared ? 'uses the link above' : 'Discord link (optional)'}
                        aria-label={`${c.name} proof link`}
                        value={row.proof}
                        onChange={(e) => setRow(c.id, { proof: e.target.value })}
                      />
                    </div>
                  );
                })}
              </fieldset>
            ))}
        </>
      )}

      <div className="challenges-bar challenges-submit-bar">
        <div>
          {chosen.length === 0 && errors.length === 0
            ? 'Nothing entered yet.'
            : `${chosen.length} ${chosen.length === 1 ? 'entry' : 'entries'} · ${chosen.filter((e) => e.proof_url !== '').length} with proof`}
          {errors.length > 0 && (
            <ul className="challenges-errors" role="alert">
              {errors.slice(0, 5).map((e) => (
                <li key={e}>{e}</li>
              ))}
            </ul>
          )}
          {status && (
            <p className={status.ok ? 'challenges-ok' : 'challenges-errors'} role="status">
              {status.text}
            </p>
          )}
        </div>
        {reviewing ? (
          <div className="challenges-bar-group">
            <button type="button" className="btn-antique" disabled={busy} onClick={() => setReviewing(false)}>
              ← Back to editing
            </button>
            <button type="button" className="btn-antique btn-danger" disabled={busy || chosen.length === 0 || errors.length > 0} onClick={() => void save()}>
              {busy ? 'Saving…' : 'Confirm & submit'}
            </button>
          </div>
        ) : (
          <button
            type="button"
            className="btn-antique"
            disabled={chosen.length === 0 || errors.length > 0}
            onClick={() => {
              setReviewing(true);
              window.scrollTo(0, 0);
            }}
          >
            Check entries →
          </button>
        )}
      </div>
    </>
  );
}

interface ReviewProps {
  apiUrl: string;
  name: string;
  list: ChallengeTile[];
  chosen: { challenge: string; domination: number; proof_url: string }[];
  mine: Map<string, number>;
  entries: ChallengeEntries;
  records: ChallengeHighscores;
}

/** Review shows what each entry changes, and anything that looks like a mistake, before saving. */
function Review({ name, list, chosen, mine, entries, records }: ReviewProps) {
  const others = bestsOf(entries.entries, name, true);
  const usedBy = proofOwners(entries.entries);
  const record = new Map(records.challenges.flatMap((c) => (c.record === null ? [] : [[c.id, c.record] as const])));
  const newRecords = chosen.filter((e) => e.domination >= 0 && e.domination > (record.get(e.challenge)?.domination ?? -Infinity)).length;
  return (
    <section className="rating-card challenges-card" aria-labelledby="review">
      <h2 id="review" className="rating-card-heading">
        Check your entries
      </h2>
      <p>
        Saving <strong>{chosen.length}</strong> {chosen.length === 1 ? 'entry' : 'entries'} as <strong>{name}</strong>.
        {newRecords > 0 && ` ★ ${newRecords} ${newRecords === 1 ? 'beats a record' : 'beat records'}.`}
      </p>
      <div className="table-responsive">
        <table className="leaderboard-table">
          <thead>
            <tr>
              <th scope="col">Challenge</th>
              <th scope="col" className="num">
                Domination
              </th>
              <th scope="col">Proof</th>
              <th scope="col" className="num">
                Your best
              </th>
              <th scope="col" className="num">
                Record
              </th>
              <th scope="col">Notes</th>
            </tr>
          </thead>
          <tbody>
            {chosen.map((e) => {
              const c = list.find((x) => x.id === e.challenge);
              const r = record.get(e.challenge);
              const notes = reviewNotes({
                domination: e.domination,
                proof: e.proof_url,
                best: mine.get(e.challenge),
                record: r?.domination,
                othersBest: others.get(e.challenge),
                reusedBy: e.proof_url === '' ? [] : usedBy(e.proof_url, name),
              });
              return (
                <tr key={e.challenge}>
                  <td>
                    {c?.name ?? e.challenge} {c && <span className="muted">({setLabel(c.set)})</span>}
                  </td>
                  <td className="num challenges-score">{e.domination}</td>
                  <td>{e.proof_url === '' ? <span className="challenge-unverified">none</span> : <ProofLink url={e.proof_url} />}</td>
                  <td className="num muted">{mine.get(e.challenge) ?? '—'}</td>
                  <td className="num muted" title={r ? r.player : 'No verified record yet'}>
                    {r?.domination ?? '—'}
                  </td>
                  <td>
                    {notes.map((n) => (
                      <span key={n.text} className={`challenges-note ${n.kind}`}>
                        {n.text}
                      </span>
                    ))}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}

/** Examples shows what to post in Discord for a verified entry. */
function Examples() {
  return (
    <>
      <div className="analysis-hero-card challenges-hero">
        <div>
          <h1 className="analysis-hero-title">Submit domination</h1>
          <p className="analysis-hero-desc">
            Enter your <strong>domination</strong> for any challenges you like: your final score minus the score of the second-place player. Fill in only the
            ones you want to add or improve. <strong>To get an entry verified</strong>, post the screenshots below in the TtA Discord&apos;s challenge channel,
            then right-click the message → <em>Copy Message Link</em> and paste it here. Entries without a link count under &ldquo;All entries&rdquo;, not in
            the official highscores.
          </p>
        </div>
      </div>
      <section className="rating-card challenges-card" aria-labelledby="examples">
        <h2 id="examples" className="rating-card-heading">
          What to post in Discord
        </h2>
        <div className="challenges-examples">
          <figure>
            <a href="/img/challenge-examples/result.jpg" target="_blank" rel="noopener">
              <img src="/img/challenge-examples/result.jpg" alt="Example: the challenge results screen" width={1375} height={822} />
            </a>
            <figcaption>
              <strong>1. Challenge results</strong>: the final scores, with your civilization behind. Domination here: 374 − 293 = <strong>81</strong>.
            </figcaption>
          </figure>
          <figure>
            <a href="/img/challenge-examples/stats.jpg" target="_blank" rel="noopener">
              <img src="/img/challenge-examples/stats.jpg" alt="Example: the culture graph with every player's leaders and wonders" width={1373} height={866} />
            </a>
            <figcaption>
              <strong>2. Statistics</strong>: the culture graph with every player&apos;s leaders and wonders.
            </figcaption>
          </figure>
          <figure>
            <div className="challenges-example-more" aria-hidden="true">
              ?
            </div>
            <figcaption>
              <strong>3. Anything else</strong>: post more if you like. The challenges live from discussion!
            </figcaption>
          </figure>
        </div>
      </section>
    </>
  );
}
