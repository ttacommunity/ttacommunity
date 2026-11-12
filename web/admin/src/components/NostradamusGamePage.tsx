/** A Nostradamus game's page, in three tabs: its rounds (drafted with their questions, published until a deadline, outcomes set, bots answered for, closed), its members (requests answered, people and bots added, linked to TTA usernames), and the standings. */
import { useEffect, useState, type FormEvent } from 'react';
import BackLink from '@ttac/shared/BackLink';
import SubTabs, { useHash } from '@ttac/shared/SubTabs';
import { hashTab } from '@ttac/shared/tabs';
import type { components } from '@ttac/shared/api';
import type { Load } from '@ttac/shared/load';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import { adminRequest, firstError, type AdminMe } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { tabsFor } from '../lib/admin-tabs';
import { closeBlocker, emptyQuestionForm, linkChoices, outcomeText, outcomeValues, questionBody, questionFormFrom, countOrNone, roundLabel, roundStatus, tokensText, toggled, withChoice, type QuestionForm } from '../lib/nostradamus-admin';
import { deadlineForm, type DeadlineForm } from '../lib/blitz-deadlines';
import { useDialog } from '@ttac/shared/Dialog';
import { countryNames, type Country } from '../lib/countries';
import { entrantToAdd } from '../lib/person-form';
import type { Region } from '../lib/zones';
import { loadFailure, outcomeOf, shownAt, type Outcome } from '../lib/outcome';
import AdminHeader from './AdminHeader';
import Notice from './Notice';
import PageError from './PageError';
import PersonForm from './PersonForm';
import PickBox from './PickBox';
import usePeopleSearch from './usePeopleSearch';
import ZonePick from './ZonePick';
import './AdminApp.css';

type Game = components['schemas']['AdminNostradamusGame'];
type Round = components['schemas']['NostradamusRound'];
type Question = components['schemas']['NostradamusQuestion'];
type Answers = components['schemas']['AdminNostradamusAnswers'];
type View = 'rounds' | 'members' | 'standings';
/** say shows an action's outcome at its place on the page, replacing the last one; null closes it. */
type Props = { game: Game; onReload: () => Promise<void>; outcome: Outcome | null; say: (o: Outcome | null) => void };

const STATUS_LABEL = { draft: 'Draft', open: 'Open for answers', awaiting: 'Awaiting outcomes', closed: 'Closed: results out' } as const;

/** NostradamusGamePage loads who is signed in and the game the address names (?edition=), then its tabs. */
export default function NostradamusGamePage() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [game, setGame] = useState<Load<Game>>({ kind: 'loading' });
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [regions, setRegions] = useState<Region[]>([]);
  const [countries, setCountries] = useState<Country[]>([]);
  // The address is read once mounted: the page is prerendered, where there is no window.
  const load = async () => {
    const slug = new URLSearchParams(window.location.search).get('edition') ?? '';
    if (slug === '') {
      setGame({ kind: 'error', message: 'This address names no Nostradamus game. Go back to the Nostradamus tab.' });
      return;
    }
    const r = await adminRequest<Game>(`/api/nostradamus/${encodeURIComponent(slug)}`, {});
    setGame(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) => setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }));
    void adminRequest<Region[]>('/api/regions', {}).then((r) => (r.ok ? setRegions(r.data) : setOutcome(loadFailure(r.message, 'page'))));
    void adminRequest<string[]>('/api/countries', {}).then((r) => (r.ok ? setCountries(countryNames(r.data)) : setOutcome(loadFailure(r.message, 'page'))));
    void load();
  }, []);
  const error = firstError(me, game);
  const [hash, setHash] = useHash();

  return (
    <>
      <AdminHeader tabs={me.kind === 'ready' ? tabsFor(me.data.role) : []} current="nostradamus" />
      <main className="admin-main">
        {error !== null && <PageError message={error} />}
        {game.kind === 'loading' && <p className="muted">Loading…</p>}
        {game.kind === 'ready' && (
          <>
            <section className="rating-card admin-card page-header" aria-labelledby="page-title">
              <div className="header-back">
                <BackLink href="/#nostradamus">Nostradamus games</BackLink>
              </div>
              <h2 id="page-title" className="tournament-title">
                {game.data.name}
              </h2>
              <p className="muted">
                Over {game.data.series} {game.data.tournament}. {game.data.rounds.length} rounds,{' '}
                {game.data.participants.filter((p) => p.status === 'accepted').length} playing, {game.data.participants.filter((p) => p.status === 'requested').length}{' '}
                asking to.
              </p>
            </section>
            <Notice outcome={shownAt(outcome, 'page')} />
            <section className="rating-card admin-card" aria-label="Rounds, members and standings">
              <SubTabs<View>
                label="Show"
                tabs={[
                  { value: 'rounds', label: 'Rounds', panel: <RoundsTab game={game.data} regions={regions} onReload={load} outcome={outcome} say={setOutcome} /> },
                  { value: 'members', label: 'Members', panel: <MembersTab game={game.data} regions={regions} countries={countries} onReload={load} outcome={outcome} say={setOutcome} /> },
                  { value: 'standings', label: 'Standings', panel: <StandingsTab game={game.data} /> },
                ]}
                chosen={hashTab<View>(['rounds', 'members', 'standings'], hash)}
                onChoose={setHash}
              />
            </section>
          </>
        )}
      </main>
    </>
  );
}

/** RoundsTab drafts a new round and shows each round at its stage, newest last. */
function RoundsTab(props: Props & { regions: readonly Region[] }) {
  const { game, onReload, outcome, say } = props;
  const [draft, setDraft] = useState({ label: '', name: '', tokensMin: '', tokensMax: '3', token_points: 10 });
  const add = async (e: FormEvent) => {
    e.preventDefault();
    const label = roundLabel(draft.label, game.rounds.map((r) => r.label));
    const r = await req.send(req.addNostradamusRound(game.slug, { label, name: draft.name.trim(), tokens_min: countOrNone(draft.tokensMin), tokens_max: countOrNone(draft.tokensMax), token_points: draft.token_points }));
    say(outcomeOf(r, `Drafted ${draft.name.trim()}.`, 'new round'));
    if (r.ok) {
      setDraft({ ...draft, label: '', name: '' });
      await onReload();
    }
  };
  return (
    <>
      {game.rounds.map((r) => (
        <RoundCard key={r.id} {...props} round={r} />
      ))}
      <form className="admin-subsection" onSubmit={(e) => void add(e)}>
        <h3>New round</h3>
        <div className="admin-add">
          <label>
            Name
            <input className="input-antique" value={draft.name} maxLength={100} required placeholder="Stage 1" onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          </label>
          <label>
            Short label, for the standings (optional)
            <input className="input-antique" value={draft.label} maxLength={20} placeholder={roundLabel('', game.rounds.map((r) => r.label))}
              onChange={(e) => setDraft({ ...draft, label: e.target.value })} />
          </label>
          <label>
            Least tokens (optional)
            <input className="input-antique" type="number" min={0} max={99} placeholder="none" value={draft.tokensMin} onChange={(e) => setDraft({ ...draft, tokensMin: e.target.value })} />
          </label>
          <label>
            Most tokens (optional)
            <input className="input-antique" type="number" min={0} max={99} placeholder="no limit" value={draft.tokensMax} onChange={(e) => setDraft({ ...draft, tokensMax: e.target.value })} />
          </label>
          <label>
            Points a token
            <input className="input-antique" type="number" min={0} max={100} value={draft.token_points} onChange={(e) => setDraft({ ...draft, token_points: Number(e.target.value) })} />
          </label>
          <button type="submit" className="btn-antique">
            Draft the round
          </button>
        </div>
        <Notice outcome={shownAt(outcome, 'new round')} onClose={() => say(null)} />
      </form>
    </>
  );
}

/** RoundCard is one round, folded once closed: a draft's questions to edit and its deadline to publish by; a published round's outcomes, answers and bots; closing it. */
function RoundCard({ game, round, regions, onReload, outcome, say }: Props & { round: Round; regions: readonly Region[] }) {
  const status = roundStatus(round, new Date());
  const at = `round ${round.id}`;
  const zone = viewerZone();
  const run = async (r: req.Request, done: string) => {
    const result = await req.send(r);
    say(outcomeOf(result, done, at));
    if (result.ok) await onReload();
  };
  const blocker = closeBlocker(round, new Date());
  const dialog = useDialog();
  const deadline = deadlineForm(regions, round.closes_at ? { at: round.closes_at, zone: round.zone } : null, zone);
  const unpublish = async (opener: HTMLElement) => {
    const sure = await dialog.confirm(
      { heading: `Unpublish ${round.name}?`, text: 'Players stop seeing it until you publish it again. Their answers are kept, except to a question whose kind, picks or choices you change.', action: 'Unpublish' },
      opener,
    );
    if (sure) await run(req.unpublishNostradamusRound(game.slug, round.id), `${round.name} is a draft again: fix it, then publish it.`);
  };
  return (
    <details className="admin-subsection" open={status !== 'closed'}>
      {dialog.element}
      <summary>
        <h3>
          {round.name} <span className="muted">({round.label})</span> · {STATUS_LABEL[status]}
        </h3>
      </summary>
      <p className="muted">
        {tokensText(round.tokens_min, round.tokens_max, round.token_points)}.{round.closes_at && ` Deadline ${formatInstant(round.closes_at, zone)}, set in ${round.zone}.`}
      </p>
      <Notice outcome={shownAt(outcome, at)} onClose={() => say(null)} />
      {round.questions.map((q) => (
        <QuestionRow key={q.id} game={game} round={round} question={q} status={status} run={run} />
      ))}
      {status === 'draft' && (
        <>
          <QuestionEditor title="Add a question" start={emptyQuestionForm} onSave={(f) => run(req.addNostradamusQuestion(game.slug, round.id, questionBody(f)), `Added ${f.label.trim()}.`)} />
          <DeadlineEditor regions={regions} start={deadline} title="Publish" submitLabel="Publish the round"
            note="Players can answer until the deadline. The bots answer what their rules can as it publishes; to change a question later, unpublish it."
            onSave={(d) => run(req.publishNostradamusRound(game.slug, round.id, d), `Published ${round.name}.`)} />
          <p>
            <button type="button" className="btn-antique" onClick={() => void run(req.removeNostradamusRound(game.slug, round.id), `Removed ${round.name}.`)}>
              Remove this draft
            </button>
          </p>
        </>
      )}
      {status !== 'draft' && <AnswersView game={game} round={round} run={run} />}
      {(status === 'open' || status === 'awaiting') && (
        <>
          <DeadlineEditor regions={regions} start={deadline} title="Deadline" submitLabel="Move the deadline" note="Moving it keeps every answer."
            onSave={(d) => run(req.setNostradamusDeadline(game.slug, round.id, d), `Moved the deadline of ${round.name}.`)} />
          <p>
            <button type="button" className="btn-antique" onClick={(e) => void unpublish(e.currentTarget)}>
              Unpublish to edit
            </button>
          </p>
        </>
      )}
      {(status === 'open' || status === 'awaiting') && (
        <p>
          <button type="button" className="btn-antique" disabled={blocker !== ''} onClick={() => void run(req.closeNostradamusRound(game.slug, round.id), `Closed ${round.name}: its results are out.`)}>
            Close and publish results
          </button>{' '}
          {blocker !== '' && <span className="muted">{blocker}</span>}
        </p>
      )}
    </details>
  );
}

/** QuestionRow is a question: a draft's to edit or remove; a published one's outcome to set until the round closes. */
function QuestionRow({ game, round, question: q, status, run }: { game: Game; round: Round; question: Question; status: string; run: (r: req.Request, done: string) => Promise<void> }) {
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(outcomeText(q.outcome));
  const [ticked, setTicked] = useState<string[]>(q.outcome ?? []);
  const fromChoices = q.kind !== 'number' && q.choices.length > 0;
  const saved = outcomeText(q.outcome);
  const choices = q.choices.map((c) => c.label + (c.seed > 0 ? ` (seed ${c.seed})` : '') + (c.leaderboard_rank > 0 ? ` (#${c.leaderboard_rank})` : '')).join(', ');
  return (
    <div className="badge-row">
      <p>
        <strong>{q.label}</strong>: {q.prompt} <span className="muted">({q.kind}{q.kind === 'set' ? ` of ${q.picks}` : ''}, {q.points} points{q.near_points > 0 ? `, ${q.near_points} one off` : ''}{q.asks_last ? ', asks who is last' : ''})</span>
      </p>
      {choices !== '' && <p className="muted">Choices: {choices}</p>}
      {status === 'draft' && (
        <p>
          <button type="button" className="btn-antique" onClick={() => setEditing(!editing)}>
            {editing ? 'Stop editing' : 'Edit'}
          </button>{' '}
          <button type="button" className="btn-antique" onClick={() => void run(req.removeNostradamusQuestion(game.slug, q.id), `Removed ${q.label}.`)}>
            Remove
          </button>
        </p>
      )}
      {editing && <QuestionEditor title={`Edit ${q.label}`} start={questionFormFrom(q)} onSave={(f) => run(req.setNostradamusQuestion(game.slug, q.id, questionBody(f)), `Saved ${f.label.trim()}.`)} />}
      {(status === 'open' || status === 'awaiting') && fromChoices && (
        <fieldset className="admin-add">
          <legend>Outcome {q.kind === 'set' ? `(the ${q.picks} that happened)` : '(tick more than one for a tie)'}</legend>
          {q.choices.map((c) => (
            <label key={c.label}>
              <input type="checkbox" checked={ticked.includes(c.label)} onChange={() => setTicked(toggled(ticked, c.label, q.kind === 'set' ? q.picks : 0))} /> {c.label}
            </label>
          ))}
          <button type="button" className="btn-antique" disabled={ticked.join(', ') === saved}
            onClick={() => void run(req.setNostradamusOutcome(game.slug, q.id, ticked), `Saved the outcome of ${q.label}.`)}>
            Save outcome
          </button>
        </fieldset>
      )}
      {(status === 'open' || status === 'awaiting') && !fromChoices && (
        <div className="admin-add">
          <label>
            Outcome {q.kind === 'number' ? '(a whole number)' : '(names separated by commas; a tie lists each)'}
            <input className="input-antique" aria-label={`Outcome: ${round.label} ${q.label}`} value={text} maxLength={500} onChange={(e) => setText(e.target.value)} />
          </label>
          <button type="button" className="btn-antique" disabled={text === outcomeText(q.outcome)}
            onClick={() => void run(req.setNostradamusOutcome(game.slug, q.id, outcomeValues(q.kind, text)), `Saved the outcome of ${q.label}.`)}>
            Save outcome
          </button>
        </div>
      )}
      {status === 'closed' && <p>Outcome: {outcomeText(q.outcome)}</p>}
    </div>
  );
}

/** QuestionEditor is the question form: its label, prompt, kind, points, and its choices one a line ("name | seed | leaderboard rank"). */
function QuestionEditor({ title, start, onSave }: { title: string; start: QuestionForm; onSave: (f: QuestionForm) => Promise<void> }) {
  const [f, setF] = useState<QuestionForm>(start);
  const [typed, setTyped] = useState('');
  const found = usePeopleSearch('/api/people', typed, false, f.choices);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    await onSave(f);
    if (start === emptyQuestionForm) setF(emptyQuestionForm);
  };
  return (
    <form className="admin-subsection" onSubmit={(e) => void save(e)}>
      <h4>{title}</h4>
      <div className="admin-add">
        <label>
          Label
          <input className="input-antique" value={f.label} maxLength={60} required onChange={(e) => setF({ ...f, label: e.target.value })} />
        </label>
        <label className="title-reason">
          Question
          <input className="input-antique" value={f.prompt} maxLength={500} onChange={(e) => setF({ ...f, prompt: e.target.value })} />
        </label>
        <label>
          Kind
          <select className="input-antique" value={f.kind} onChange={(e) => setF({ ...f, kind: e.target.value as QuestionForm['kind'] })}>
            <option value="pick">Pick one</option>
            <option value="set">Pick several, all to be right</option>
            <option value="number">A number</option>
          </select>
        </label>
        <label>
          Points
          <input className="input-antique" type="number" min={0} max={100} value={f.points} onChange={(e) => setF({ ...f, points: Number(e.target.value) })} />
        </label>
        {f.kind === 'number' && (
          <label>
            Points one off
            <input className="input-antique" type="number" min={0} max={100} value={f.near_points} onChange={(e) => setF({ ...f, near_points: Number(e.target.value) })} />
          </label>
        )}
        {f.kind === 'set' && (
          <label>
            How many
            <input className="input-antique" type="number" min={1} max={20} value={f.picks} onChange={(e) => setF({ ...f, picks: Number(e.target.value) })} />
          </label>
        )}
        {f.kind !== 'number' && (
          <label>
            <input type="checkbox" checked={f.asks_last} onChange={(e) => setF({ ...f, asks_last: e.target.checked })} /> Asks who finishes last
          </label>
        )}
      </div>
      {f.kind !== 'number' && (
        <PickBox label="Add a player to the choices" text={typed} placeholder="Search a TTA username" options={linkChoices(found, typed)}
          onText={setTyped} onPick={(o) => { setF({ ...f, choices: withChoice(f.choices, o.label) }); setTyped(''); }} />
      )}
      {f.kind !== 'number' && (
        <label className="admin-block">
          Choices, one a line: name | seed | leaderboard rank (both optional; the bots use them)
          <textarea className="input-antique" rows={5} value={f.choices} onChange={(e) => setF({ ...f, choices: e.target.value })} />
        </label>
      )}
      <button type="submit" className="btn-antique">
        Save question
      </button>
    </form>
  );
}

/** DeadlineEditor sets a round's deadline, a day and time on the clocks of a zone: to publish a draft, or to move an open round's. */
function DeadlineEditor({ regions, start, title, note, submitLabel, onSave }: {
  regions: readonly Region[];
  start: DeadlineForm;
  title: string;
  note: string;
  submitLabel: string;
  onSave: (d: { day: string; time: string; zone: string }) => Promise<void>;
}) {
  const [d, setD] = useState<DeadlineForm>(start.time === '' ? { ...start, time: '18:00' } : start);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    await onSave({ ...d, zone: d.zone.trim() });
  };
  return (
    <form className="admin-subsection" onSubmit={(e) => void save(e)}>
      <h4>{title}</h4>
      <p className="muted">{note}</p>
      <div className="admin-add">
        <label>
          Day
          <input className="input-antique" type="date" required value={d.day} onChange={(e) => setD({ ...d, day: e.target.value })} />
        </label>
        <label>
          Time
          <input className="input-antique" type="time" required value={d.time} onChange={(e) => setD({ ...d, time: e.target.value })} />
        </label>
        <ZonePick label="Time zone" regions={regions} value={d.zone} onChange={(zone) => setD({ ...d, zone })} />
        <button type="submit" className="btn-antique">
          {submitLabel}
        </button>
      </div>
    </form>
  );
}

/** AnswersView shows every answer in a published round, by participant, with each bot's answers to give while the round is open. */
function AnswersView({ game, round, run }: { game: Game; round: Round; run: (r: req.Request, done: string) => Promise<void> }) {
  const [answers, setAnswers] = useState<Load<Answers> | null>(null);
  const load = async () => {
    setAnswers({ kind: 'loading' });
    const r = await adminRequest<Answers>(`/api/nostradamus/${encodeURIComponent(game.slug)}/rounds/${round.id}/answers`, {});
    setAnswers(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  const labelOf = (id: number) => round.questions.find((q) => q.id === id)?.label ?? String(id);
  const bots = game.participants.filter((p) => p.bot && p.status === 'accepted');
  return (
    <>
      <p>
        <button type="button" className="btn-antique" aria-expanded={answers !== null} onClick={() => (answers === null ? void load() : setAnswers(null))}>
          {answers === null ? 'Show the answers' : 'Hide the answers'}
        </button>
      </p>
      {answers?.kind === 'error' && <p role="alert">{answers.message}</p>}
      {answers?.kind === 'ready' && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Every answer in {round.name}</caption>
            <thead>
              <tr>
                <th scope="col">Player</th>
                <th scope="col">Question</th>
                <th scope="col">Answer</th>
                <th scope="col" className="num">
                  Tokens
                </th>
                <th scope="col" className="num">
                  Points
                </th>
              </tr>
            </thead>
            <tbody>
              {answers.data.answers.map((a) => (
                <tr key={`${a.participant}-${a.question}`}>
                  <td>{a.participant}</td>
                  <td>{labelOf(a.question)}</td>
                  <td>{a.answer.join(', ')}</td>
                  <td className="num">{a.tokens > 0 ? a.tokens : ''}</td>
                  <td className="num">{a.points ?? '–'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {roundStatus(round, new Date()) === 'open' &&
        answers?.kind === 'ready' &&
        bots.map((b) => (
          <BotAnswers key={b.id} bot={b} round={round} saved={answers.data.answers.filter((a) => a.participant === b.name)}
            onSave={(given) => run(req.setNostradamusBotAnswers(game.slug, round.id, b.id, given), `Saved ${b.name}'s answers.`).then(load)} />
        ))}
    </>
  );
}

/** BotAnswers gives a bot's answers to an open round by hand, for the questions its rule cannot answer: names separated by commas. */
function BotAnswers({ bot, round, saved, onSave }: { bot: Game['participants'][number]; round: Round; saved: Answers['answers']; onSave: (given: components['schemas']['NostradamusGiven'][]) => Promise<void> }) {
  const [texts, setTexts] = useState<Record<number, string>>(() =>
    Object.fromEntries(round.questions.map((q) => [q.id, saved.find((a) => a.question === q.id)?.answer.join(', ') ?? ''])),
  );
  const [picked, setPicked] = useState<Record<number, string[]>>(() =>
    Object.fromEntries(round.questions.map((q) => [q.id, saved.find((a) => a.question === q.id)?.answer ?? []])),
  );
  const fromChoices = (q: Question) => q.kind !== 'number' && q.choices.length > 0;
  const [tokens, setTokens] = useState<Record<number, number>>(() => Object.fromEntries(round.questions.map((q) => [q.id, saved.find((a) => a.question === q.id)?.tokens ?? 0])));
  const given = round.questions.flatMap((q) => {
    const answer = fromChoices(q) ? (picked[q.id] ?? []) : outcomeValues(q.kind, texts[q.id] ?? '');
    return answer.length === 0 ? [] : [{ question: q.id, answer, tokens: tokens[q.id] ?? 0 }];
  });
  return (
    <div className="admin-subsection">
      <h4>
        {bot.name}&apos;s answers {bot.bot_rule !== '' && <span className="muted">(answers by {bot.bot_rule} where it can)</span>}
      </h4>
      <div className="admin-add">
        {round.questions.map((q) => (
          <label key={q.id}>
            {q.label}
            {fromChoices(q) ? (
              <span className="admin-add">
                {q.choices.map((c) => (
                  <span key={c.label}>
                    <input type={q.kind === 'set' ? 'checkbox' : 'radio'} name={`bot-${bot.id}-${q.id}`} aria-label={`${bot.name}, ${q.label}: ${c.label}`}
                      checked={(picked[q.id] ?? []).includes(c.label)}
                      onChange={() => setPicked({ ...picked, [q.id]: toggled(picked[q.id] ?? [], c.label, q.kind === 'set' ? q.picks : 1) })} /> {c.label}
                  </span>
                ))}
              </span>
            ) : (
              <input className="input-antique" value={texts[q.id] ?? ''} onChange={(e) => setTexts({ ...texts, [q.id]: e.target.value })} />
            )}
            <span>
              Tokens{' '}
              <input className="input-antique" type="number" min={0} max={round.tokens_max ?? 99} aria-label={`${bot.name}, tokens on ${q.label}`} value={tokens[q.id] ?? 0}
                onChange={(e) => setTokens({ ...tokens, [q.id]: Math.max(0, Math.trunc(Number(e.target.value))) })} />
            </span>
          </label>
        ))}
      </div>
      <button type="button" className="btn-antique" onClick={() => void onSave(given)}>
        Save {bot.name}&apos;s answers
      </button>
    </div>
  );
}

/** MembersTab answers the requests to play, adds people and bots, and links each participant to a TTA username. */
function MembersTab({ game, regions, countries, onReload, outcome, say }: Props & { regions: readonly Region[]; countries: readonly Country[] }) {
  const [bot, setBot] = useState<{ name: string; bot_rule: 'seed' | 'glicko' | 'leaderboard' }>({ name: '', bot_rule: 'glicko' });
  const run = async (r: req.Request, done: string, at: string) => {
    const result = await req.send(r);
    say(outcomeOf(result, done, at));
    if (result.ok) await onReload();
  };
  const requests = game.participants.filter((p) => p.status === 'requested');
  return (
    <>
      <div className="admin-subsection">
        <h3>Asking to play ({requests.length})</h3>
        <Notice outcome={shownAt(outcome, 'requests')} onClose={() => say(null)} />
        {requests.length === 0 && <p className="muted">Nobody is waiting.</p>}
        {requests.map((p) => (
          <p key={p.id}>
            <strong>{p.name}</strong> {p.discord_name !== '' && <span className="muted">({p.discord_name})</span>}{' '}
            <button type="button" className="btn-antique" onClick={() => void run(req.setNostradamusStatus(game.slug, p.id, 'accepted'), `Let ${p.name} in.`, 'requests')}>
              Accept
            </button>{' '}
            <button type="button" className="btn-antique" onClick={() => void run(req.setNostradamusStatus(game.slug, p.id, 'refused'), `Refused ${p.name}.`, 'requests')}>
              Refuse
            </button>
          </p>
        ))}
      </div>
      <div className="admin-subsection">
        <h3>Add a player</h3>
        <p className="muted">
          Search the members and the players the ratings know, or add someone new by their username as on CGE. What we know of them fills in; anything you add or change is saved on them. They play at once.
        </p>
        <PersonForm
          searchBase="/api/people"
          taken={{ entered: game.participants.flatMap((p) => [p.name, p.member ?? '']).filter((n) => n !== ''), membersTaken: false }}
          regions={regions}
          countries={countries}
          submitLabel="Add"
          onSubmit={async (d) => {
            const r = await req.send(req.addNostradamusParticipant(game.slug, entrantToAdd(d)));
            say(outcomeOf(r, `Added ${d.name.trim()}.`, 'add'));
            if (r.ok) await onReload();
            return r.ok;
          }}
          refresh={game}
        />
        <Notice outcome={shownAt(outcome, 'add')} onClose={() => say(null)} />
      </div>
      <form className="admin-subsection" onSubmit={(e) => { e.preventDefault(); void run(req.addNostradamusParticipant(game.slug, { name: bot.name.trim(), bot_rule: bot.bot_rule }), `Added ${bot.name.trim()}.`, 'add bot').then(() => setBot({ ...bot, name: '' })); }}>
        <h3>Add a bot</h3>
        <div className="admin-add">
          <label>
            Name
            <input className="input-antique" required maxLength={100} value={bot.name} onChange={(e) => setBot({ ...bot, name: e.target.value })} />
          </label>
          <label>
            Answers by
            <select className="input-antique" value={bot.bot_rule} onChange={(e) => setBot({ ...bot, bot_rule: e.target.value as typeof bot.bot_rule })}>
              <option value="glicko">Highest Glicko</option>
              <option value="seed">Best seed</option>
              <option value="leaderboard">Best leaderboard rank</option>
            </select>
          </label>
          <button type="submit" className="btn-antique">
            Add the bot
          </button>
        </div>
        <Notice outcome={shownAt(outcome, 'add bot')} onClose={() => say(null)} />
      </form>
      <div className="admin-subsection">
        <h3>Everyone</h3>
        <Notice outcome={shownAt(outcome, 'everyone')} onClose={() => say(null)} />
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Each participant, their status, and the member they are linked to</caption>
            <thead>
              <tr>
                <th scope="col">Plays as</th>
                <th scope="col">Status</th>
                <th scope="col">Member (TTA username)</th>
              </tr>
            </thead>
            <tbody>
              {game.participants.map((p) => (
                <ParticipantRow key={p.id} game={game} participant={p} run={run} />
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

/** ParticipantRow is one participant: their status to change, and the member to link them to. */
function ParticipantRow({ game, participant: p, run }: { game: Game; participant: Game['participants'][number]; run: (r: req.Request, done: string, at: string) => Promise<void> }) {
  const [member, setMember] = useState(p.member ?? '');
  const found = usePeopleSearch('/api/people', member, member === (p.member ?? ''), game);
  return (
    <tr>
      <td>
        {p.name}
        {p.bot && <span className="muted"> (bot{p.bot_rule !== '' ? `, ${p.bot_rule}` : ''})</span>}
        {p.discord_name !== '' && <span className="muted"> · {p.discord_name}</span>}
      </td>
      <td>
        <select className="input-antique" aria-label={`Status: ${p.name}`} value={p.status}
          onChange={(e) => void run(req.setNostradamusStatus(game.slug, p.id, e.target.value as 'accepted' | 'refused'), `Saved ${p.name}.`, 'everyone')}>
          {p.status === 'requested' && <option value="requested">Asking</option>}
          <option value="accepted">Playing</option>
          <option value="refused">Refused</option>
        </select>
      </td>
      <td>
        {!p.bot && (
          <>
            <PickBox label={`Member: ${p.name}`} hideLabel text={member} placeholder="Search a TTA username" options={linkChoices(found, member)}
              onText={setMember} onPick={(o) => setMember(o.label)} />{' '}
            <button type="button" className="btn-antique" disabled={member === (p.member ?? '')}
              onClick={() => void run(req.linkNostradamusParticipant(game.slug, p.id, member), member.trim() === '' ? `Unlinked ${p.name}.` : `Linked ${p.name} to ${member.trim()}.`, 'everyone')}>
              Save
            </button>
          </>
        )}
      </td>
    </tr>
  );
}

/** StandingsTab shows the standings over every round with an outcome, closed or not: what the site will show once each closes. */
function StandingsTab({ game }: { game: Game }) {
  return (
    <div className="admin-subsection">
      <p className="muted">Every round with an outcome counts here; the site shows only closed rounds.</p>
      {game.standings.length === 0 ? (
        <p className="muted">No round has an outcome yet.</p>
      ) : (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Each participant's points by round, and their total</caption>
            <thead>
              <tr>
                <th scope="col">Player</th>
                {game.scored.map((r) => (
                  <th key={r} scope="col" className="num">
                    {r}
                  </th>
                ))}
                <th scope="col" className="num">
                  Total
                </th>
              </tr>
            </thead>
            <tbody>
              {game.standings.map((s) => (
                <tr key={s.name}>
                  <td>
                    {s.member ?? s.name}
                    {s.bot && <span className="muted"> (bot)</span>}
                  </td>
                  {s.rounds.map((r) => (
                    <td key={r.round} className={r.missed ? 'num muted' : 'num'}>
                      {r.points}
                    </td>
                  ))}
                  <td className="num">
                    <strong>{s.total}</strong>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
