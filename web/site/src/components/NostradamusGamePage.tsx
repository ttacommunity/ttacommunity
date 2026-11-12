/** The Nostradamus section: the games (active ones, or past ones too), or one game (?edition=) with the player's place, its standings, its rounds one at a time, and its players. */
import { useState } from 'react';
import BackLink from '@ttac/shared/BackLink';
import SubTabs, { useHash } from '@ttac/shared/SubTabs';
import { hashTab } from '@ttac/shared/tabs';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import { nostradamusEditions, nostradamusGame, type NostradamusGame, type NostradamusQuestion, type NostradamusRound } from '@ttac/shared/ratings';
import { browserStorage, seemsSignedIn } from '../lib/account';
import { askToPlay, fetchMyNostradamus, sendAnswers, type MyPlace } from '../lib/nostradamus-me';
import {
  answeredOf,
  choose,
  draftFrom,
  editionParam,
  gameStatus,
  givenFrom,
  joinState,
  nostradamusHref,
  openUntil,
  places,
  pollShare,
  roundLabel,
  roundStage,
  shownGames,
  shownName,
  startingRound,
  timeLeft,
  stakeRange,
  staked,
  tokensShort,
  withStake,
  valuesText,
  type Draft,
} from '../lib/nostradamus';
import { useKeptLoad } from './ChallengeParts';
import { useLoad } from './useLoad';
import Waiting from './Waiting';
import './Blitz.css';

const VIEWS = ['standings', 'rounds', 'players'] as const;
const RULES =
  "The community's prediction game for the World Championship. Each round, players predict the results before the deadline and may stake tokens on the answers they are surest of: each token on a right answer wins its points, on a wrong one loses them. A round missed scores the lowest of the bots.";

/** NostradamusGamePage shows the game the address names, or the list of games. */
export default function NostradamusGamePage({ apiUrl }: { apiUrl: string }) {
  const [edition] = useState(() => editionParam(window.location.search));
  return edition === null ? <GamesList apiUrl={apiUrl} /> : <GamePage apiUrl={apiUrl} edition={edition} />;
}

/** GamesList lists the games still running, and the past ones too when asked. */
function GamesList({ apiUrl }: { apiUrl: string }) {
  const [past, setPast] = useState(false);
  const editions = useLoad('editions', () => nostradamusEditions(apiUrl));
  const now = new Date();
  return (
    <>
      <div className="tournaments-header">
        <h1 className="tournaments-title">
          <span>🔮</span> Nostradamus
        </h1>
        <div className="tournaments-subtitle">{RULES}</div>
      </div>
      {editions.kind !== 'ready' ? (
        <Waiting load={editions} what="the games" />
      ) : (
        <section className="rating-card blitz-card" aria-labelledby="nostradamus-games">
          <h2 id="nostradamus-games" className="rating-card-heading">
            Games
          </h2>
          <div className="blitz-table-bar">
            <span>{past ? 'Every game, newest first.' : 'The games still running, newest first.'}</span>
            <label className="blitz-toggle">
              <input type="checkbox" checked={past} onChange={(e) => setPast(e.target.checked)} /> Show past games
            </label>
          </div>
          {shownGames(editions.data.editions, past).length === 0 ? (
            <p className="muted">{past ? 'No game yet.' : 'No game is running.'}</p>
          ) : (
            <div className="blitz-table-wrap">
              <table className="blitz-table">
                <caption className="visually-hidden">Nostradamus games, newest first</caption>
                <thead>
                  <tr>
                    <th scope="col">Game</th>
                    <th scope="col">Tournament</th>
                    <th scope="col">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {shownGames(editions.data.editions, past).map((g) => (
                    <tr key={g.slug}>
                      <th scope="row">
                        <a href={nostradamusHref(g.slug)}>{g.name}</a>
                      </th>
                      <td>{g.tournament}</td>
                      <td>{gameStatus(g, now)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      )}
    </>
  );
}

/** GamePage is one game: its header, the player's place, and tabs for its standings, its rounds (one shown at a time, the open or latest first) and its players. */
function GamePage({ apiUrl, edition }: { apiUrl: string; edition: string }) {
  const [asked, setAsked] = useState(0);
  const [hash, setHash] = useHash();
  const [round, setRound] = useState<number | null>(null);
  const game = useKeptLoad(`game-${asked}`, () => nostradamusGame(apiUrl, edition));
  const place = useKeptLoad(`mine-${asked}`, () => myPlace(apiUrl, edition));
  const reload = () => setAsked((n) => n + 1);
  const back = (
    <div className="header-back">
      <BackLink href="/nostradamus/">Every Nostradamus game</BackLink>
    </div>
  );
  if (game.kind !== 'ready') {
    return (
      <>
        <div className="tournaments-header">
          {back}
          <h1 className="tournaments-title">
            <span>🔮</span> Nostradamus
          </h1>
        </div>
        <Waiting load={game} what="the game" />
      </>
    );
  }
  const g = game.data;
  const mine = place.kind === 'ready' ? place.data : null;
  const shown = round ?? startingRound(g.rounds, new Date());
  const openRound = (id: number) => {
    setRound(id);
    setHash('rounds');
  };
  const panels = {
    standings: { label: 'Standings', panel: <Standings game={g} /> },
    rounds: { label: 'Rounds', panel: <RoundsPanel apiUrl={apiUrl} game={g} place={mine} shown={shown} onPick={setRound} onSaved={reload} /> },
    players: { label: 'Players', panel: <PlayersPanel game={g} /> },
  };
  return (
    <>
      <div className="tournaments-header">
        {back}
        <h1 className="tournaments-title">
          <span>🔮</span> {g.name}
        </h1>
        <div className="tournaments-subtitle">
          {g.tournament}. {gameStatus({ finished: g.finished, open_until: openUntil(g.rounds, new Date()) }, new Date())}.
        </div>
        <div className="tournaments-subtitle">{RULES}</div>
      </div>
      <JoinBox apiUrl={apiUrl} game={g} place={mine} onChange={reload} onOpen={openRound} />
      <section className="rating-card blitz-card" aria-label="Standings, rounds and players">
        <SubTabs label="Show" tabs={VIEWS.map((v) => ({ value: v, ...panels[v] }))} chosen={hashTab(VIEWS, hash)} onChoose={setHash} />
      </section>
    </>
  );
}

/** myPlace reads the player's place in the game, asking the server only when someone seems signed in. */
function myPlace(apiUrl: string, edition: string): Promise<MyPlace> {
  return seemsSignedIn(browserStorage()) ? fetchMyNostradamus(apiUrl, edition) : Promise.resolve({ kind: 'signed-out' });
}

/** JoinBox tells the player where they stand (sign in, link a TTA username, ask to play, waiting, refused, or in) and, once in, how much of each open round they have answered. */
function JoinBox({ apiUrl, game, place, onChange, onOpen }: { apiUrl: string; game: NostradamusGame; place: MyPlace | null; onChange: () => void; onOpen: (round: number) => void }) {
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const state = place === null ? null : joinState(place.kind !== 'signed-out', place.kind === 'signed-in', place.kind === 'signed-in' ? place.mine.status : 'none');
  const ask = async () => {
    setBusy(true);
    const r = await askToPlay(apiUrl, game.slug);
    setBusy(false);
    if (r.ok) onChange();
    else setMessage(r.message);
  };
  return (
    <section className="rating-card blitz-card" aria-labelledby="nostradamus-join">
      <h2 id="nostradamus-join" className="rating-card-heading">
        Your play
      </h2>
      {state === 'sign-in' && (
        <p>
          <a href="/account/">Sign in</a> to ask to play.
        </p>
      )}
      {state === 'link' && (
        <p>
          Link your account to your TTA username on <a href="/account/">your account page</a>, then ask to play here.
        </p>
      )}
      {state === 'ask' && (
        <p>
          <button type="button" className="btn-antique" disabled={busy} onClick={() => void ask()}>
            Ask to play
          </button>{' '}
          <span className="muted">An organiser lets you in.</span>
        </p>
      )}
      {state === 'requested' && <p>You have asked to play: an organiser will let you in.</p>}
      {state === 'refused' && <p>An organiser did not let you into this game. Ask them on Discord if you think that is wrong.</p>}
      {state === 'play' && <p>You are in. Answer each round before its deadline; you can change your answers until then.</p>}
      {state === 'play' &&
        place?.kind === 'signed-in' &&
        game.rounds
          .filter((r) => roundStage(r, new Date()) === 'open')
          .map((r) => {
            const done = answeredOf(r.questions, place.mine.answers);
            return (
              <p key={r.id} className={done.answered < done.of ? 'nostradamus-nudge' : undefined}>
                <strong>{r.name}</strong> is open, {r.closes_at ? timeLeft(r.closes_at, new Date()) : ''}: you have answered {done.answered} of {done.of}.{' '}
                <button type="button" className="blitz-text-button" onClick={() => onOpen(r.id)}>
                  {done.answered < done.of ? 'Answer now' : 'Change your answers'}
                </button>
              </p>
            );
          })}
      {message !== '' && <p role="alert">{message}</p>}
    </section>
  );
}

/** RoundsPanel shows one published round, picked from a list: its answer form for an accepted player while it is open, else its questions and, once closed, its results. */
function RoundsPanel({ apiUrl, game, place, shown, onPick, onSaved }: { apiUrl: string; game: NostradamusGame; place: MyPlace | null; shown: number | null; onPick: (round: number) => void; onSaved: () => void }) {
  const now = new Date();
  const round = game.rounds.find((r) => r.id === shown);
  if (round === undefined) return <p className="muted">No round is published yet.</p>;
  const answering = roundStage(round, now) === 'open' && place?.kind === 'signed-in' && place.mine.status === 'accepted';
  return (
    <>
      <div className="blitz-table-bar">
        <span className="nostradamus-round-pick">
          <label htmlFor="nostradamus-round">Round</label>
          <select id="nostradamus-round" className="input-antique" value={round.id} onChange={(e) => onPick(Number(e.target.value))}>
            {[...game.rounds].reverse().map((r) => (
              <option key={r.id} value={r.id}>
                {roundLabel(r, now)}
              </option>
            ))}
          </select>
        </span>
      </div>
      {answering ? <AnswerForm key={round.id} apiUrl={apiUrl} edition={game.slug} round={round} place={place} onSaved={onSaved} /> : <RoundView round={round} place={place} />}
    </>
  );
}

/** PlayersPanel lists the game's accepted players, people before bots. */
function PlayersPanel({ game }: { game: NostradamusGame }) {
  if (game.players.length === 0) return <p className="muted">Nobody is playing yet.</p>;
  return (
    <div className="blitz-table-wrap">
      <table className="blitz-table">
        <caption className="visually-hidden">{game.name}: its players</caption>
        <thead>
          <tr>
            <th scope="col">Player</th>
            <th scope="col">TTA username</th>
          </tr>
        </thead>
        <tbody>
          {game.players.map((p) => (
            <tr key={p.name}>
              <th scope="row">
                {p.name}
                {p.bot && <span className="muted"> (bot)</span>}
              </th>
              <td>{p.member ?? ''}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** AnswerForm is an open round's questions for an accepted player, filled from their saved answers, with tokens to stake on each within the round's least and most. */
function AnswerForm({ apiUrl, edition, round, place, onSaved }: { apiUrl: string; edition: string; round: NostradamusRound; place: MyPlace; onSaved: () => void }) {
  const saved = place.kind === 'signed-in' ? place.mine.answers : [];
  const [draft, setDraft] = useState<Draft>(() => draftFrom(round.questions, saved));
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const zone = viewerZone();
  const save = async () => {
    setBusy(true);
    const r = await sendAnswers(apiUrl, edition, round.id, givenFrom(round.questions, draft));
    setBusy(false);
    setMessage(r.ok ? 'Saved. You can change your answers until the deadline.' : r.message);
    if (r.ok) onSaved();
  };
  return (
    <section aria-labelledby={`answer-${round.id}`}>
      <h2 id={`answer-${round.id}`} className="rating-card-heading">
        {round.name}: your answers
      </h2>
      <p className="muted">
        Closes {round.closes_at ? `${formatInstant(round.closes_at, zone)} (${timeLeft(round.closes_at, new Date())})` : ''}. Answered{' '}
        {givenFrom(round.questions, draft).length} of {round.questions.length}. Tokens staked: {staked(draft)}
        {round.tokens_max !== null && ` of ${round.tokens_max}`}
        {round.tokens_min !== null && round.tokens_min > 0 && ` (at least ${round.tokens_min})`}, ±{round.token_points} each.
      </p>
      {round.questions.map((q) => (
        <fieldset key={q.id} className="nostradamus-question">
          <legend>
            {q.prompt || q.label} <span className="muted">({questionWorth(q)})</span>
          </legend>
          {q.kind === 'number' || q.choices.length === 0 ? (
            <input
              className="input-antique"
              inputMode={q.kind === 'number' ? 'numeric' : 'text'}
              aria-label={`Your answer: ${q.label}`}
              value={draft[q.id]?.answer[0] ?? ''}
              onChange={(e) => setDraft(choose(draft, q, e.target.value))}
            />
          ) : (
            <div className="nostradamus-choices">
              {q.choices.map((c) => (
                <button
                  key={c.label}
                  type="button"
                  className="btn-antique nostradamus-choice"
                  aria-pressed={draft[q.id]?.answer.includes(c.label) ?? false}
                  onClick={() => setDraft(choose(draft, q, c.label))}
                >
                  {c.label}
                </button>
              ))}
            </div>
          )}
          {round.tokens_max !== 0 && <TokenStepper label={q.label} value={draft[q.id]?.tokens ?? 0} range={stakeRange(draft, q.id, round.tokens_max)}
            onChange={(n) => setDraft(withStake(draft, q.id, n, round.tokens_max))} />}
        </fieldset>
      ))}
      {tokensShort(draft, round.tokens_min) > 0 && <p className="nostradamus-nudge">Stake {tokensShort(draft, round.tokens_min)} more token{tokensShort(draft, round.tokens_min) === 1 ? '' : 's'} to save: this round asks for at least {round.tokens_min}.</p>}
      <p>
        <button type="button" className="btn-antique" disabled={busy || tokensShort(draft, round.tokens_min) > 0} onClick={() => void save()}>
          Save answers
        </button>
      </p>
      {message !== '' && <p role="status">{message}</p>}
    </section>
  );
}

/** questionWorth says what a question is worth and how many names it takes. */
function questionWorth(q: NostradamusQuestion): string {
  const picks = q.kind === 'set' ? `pick ${q.picks}, ` : '';
  const near = q.near_points > 0 ? `, ${q.near_points} if one off` : '';
  return `${picks}${q.points} points${near}`;
}

/** Standings lists each participant's points in each closed round, and their total, ties sharing a place. */
function Standings({ game }: { game: NostradamusGame }) {
  const ranks = places(game.standings.map((s) => s.total));
  return (
    <section aria-labelledby="nostradamus-standings">
      <h2 id="nostradamus-standings" className="rating-card-heading">
        Standings
      </h2>
      {game.standings.length === 0 ? (
        <p className="muted">No round has its results yet.</p>
      ) : (
        <div className="table-responsive">
          <table className="leaderboard-table">
            <caption className="visually-hidden">{game.name}: each participant&apos;s points in each round with results, and their total</caption>
            <thead>
              <tr>
                <th scope="col" className="num">
                  #
                </th>
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
              {game.standings.map((s, i) => (
                <tr key={s.name}>
                  <td className="num">{ranks[i]}</td>
                  <td>
                    {shownName(s)}
                    {s.bot && <span className="muted"> (bot)</span>}
                  </td>
                  {s.rounds.map((r) => (
                    <td key={r.round} className={r.missed ? 'num muted' : 'num'} title={r.missed ? "Missed: the lowest bot's score" : undefined}>
                      {r.points}
                      {r.missed && <span className="visually-hidden"> (missed)</span>}
                    </td>
                  ))}
                  <td className="num">
                    <strong>{s.total}</strong>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="muted">A greyed score is a round missed, scored as the lowest of the bots.</p>
        </div>
      )}
    </section>
  );
}

/** RoundView shows a published round: its questions and deadline while open; once closed, each outcome, the player's own answer, and how many picked each choice. */
function RoundView({ round, place }: { round: NostradamusRound; place: MyPlace | null }) {
  const stage = roundStage(round, new Date());
  const mine = place?.kind === 'signed-in' ? place.mine.answers : [];
  const zone = viewerZone();
  return (
    <section aria-labelledby={`round-${round.id}`}>
      <h2 id={`round-${round.id}`} className="rating-card-heading">
        {round.name}
      </h2>
      <p className="muted">
        {stage === 'open' && `Open until ${round.closes_at ? `${formatInstant(round.closes_at, zone)}, ${timeLeft(round.closes_at, new Date())}` : ''}.`}
        {stage === 'awaiting' && 'Closed for answers: results once the organisers publish them.'}
        {stage === 'closed' && 'Results are in.'}
      </p>
      {round.questions.map((q) => {
        const my = mine.find((a) => a.question === q.id);
        return (
          <div key={q.id} className="nostradamus-question">
            <h3>{q.prompt || q.label}</h3>
            {stage === 'closed' && <p>Outcome: {valuesText(q.outcome)}</p>}
            {my && (
              <p>
                Your answer: {valuesText(my.answer)}
                {my.tokens > 0 && ` (${my.tokens} token${my.tokens === 1 ? '' : 's'})`}
              </p>
            )}
            {stage === 'closed' && q.choices.length > 0 && (
              <ul className="nostradamus-poll">
                {q.choices.map((c) => {
                  const p = pollShare(q, c.label);
                  return (
                    <li key={c.label}>
                      <span>{c.label}</span> <meter min={0} max={100} value={p.share} aria-label={`${c.label}: ${p.share}%`} />{' '}
                      <span className="muted">
                        {p.answers} ({p.share}%)
                      </span>
                    </li>
                  );
                })}
              </ul>
            )}
            {stage === 'closed' && q.choices.length === 0 && q.counts.length > 0 && (
              <ul className="nostradamus-poll">
                {q.counts.map((c) => (
                  <li key={c.value}>
                    {c.value}: <span className="muted">{c.answers}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        );
      })}
    </section>
  );
}

/** TokenStepper sets the tokens staked on an answer: a number field with − and + beside it, kept within range. The field may be cleared while typing; left empty, it counts as none. */
function TokenStepper({ label, value, range, onChange }: { label: string; value: number; range: { min: number; max: number }; onChange: (n: number) => void }) {
  const [text, setText] = useState<string | null>(null);
  const type = (t: string) => {
    setText(t);
    if (/^\d+$/.test(t.trim())) onChange(Number(t));
  };
  const settle = () => {
    if (text !== null && text.trim() === '') onChange(0);
    setText(null);
  };
  return (
    <div className="nostradamus-stake">
      <span>Tokens</span>
      <button type="button" className="btn-antique" aria-label={`One token fewer: ${label}`} disabled={value <= range.min} onClick={() => onChange(value - 1)}>
        −
      </button>
      <input
        className="input-antique"
        type="text"
        inputMode="numeric"
        pattern="[0-9]*"
        aria-label={`Tokens on ${label}`}
        value={text ?? String(value)}
        onFocus={(e) => e.currentTarget.select()}
        onChange={(e) => type(e.target.value)}
        onBlur={settle}
      />
      <button type="button" className="btn-antique" aria-label={`One token more: ${label}`} disabled={value >= range.max} onClick={() => onChange(value + 1)}>
        +
      </button>
    </div>
  );
}
