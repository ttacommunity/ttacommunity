/** A Nostradamus game: the join box and the open round's questions for a signed-in player, the standings, and each published round with, once closed, its outcomes and how many picked each answer. */
import { useEffect, useState } from 'react';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import { nostradamusEditions, nostradamusGame, type NostradamusGame, type NostradamusQuestion, type NostradamusRound } from '@ttac/shared/ratings';
import { browserStorage, seemsSignedIn } from '../lib/account';
import { askToPlay, fetchMyNostradamus, sendAnswers, type MyPlace } from '../lib/nostradamus-me';
import {
  answeredOf,
  chosenEdition,
  choose,
  draftFrom,
  editionLabel,
  editionParam,
  givenFrom,
  joinState,
  nostradamusHref,
  places,
  pollShare,
  roundStage,
  shownName,
  timeLeft,
  tokensLeft,
  toggleToken,
  valuesText,
  type Draft,
} from '../lib/nostradamus';
import { useLoad } from './useLoad';
import Waiting from './Waiting';

/** NostradamusGamePage loads the games, then the chosen one and, for someone signed in, their place in it. */
export default function NostradamusGamePage({ apiUrl }: { apiUrl: string }) {
  const [wanted, setWanted] = useState<string | null | undefined>(undefined);
  const [asked, setAsked] = useState(0);
  useEffect(() => setWanted(editionParam(window.location.search)), []);
  const editions = useLoad('editions', wanted === undefined ? null : () => nostradamusEditions(apiUrl));
  const edition = editions.kind === 'ready' ? chosenEdition(editions.data.editions, wanted ?? null) : null;
  const game = useLoad(`game-${edition}-${asked}`, edition === null ? null : () => nostradamusGame(apiUrl, edition));
  const place = useLoad(`mine-${edition}-${asked}`, edition === null ? null : () => myPlace(apiUrl, edition));
  const reload = () => setAsked((n) => n + 1);

  return (
    <>
      <div className="analysis-hero-card">
        <div>
          <h1 className="analysis-hero-title">Nostradamus</h1>
          <p className="analysis-hero-desc">
            The community&apos;s prediction game for the World Championship. Each round, players predict the results before the deadline and may put tokens on
            the answers they are surest of: a right one wins the token&apos;s points, a wrong one loses them. A round missed scores the lowest of the bots.
          </p>
        </div>
      </div>
      {editions.kind !== 'ready' ? (
        <Waiting load={editions} what="the games" />
      ) : edition === null ? (
        <div className="rating-card waiting-message">No game yet.</div>
      ) : (
        <>
          {editions.data.editions.length > 1 && (
            <label className="nostradamus-pick">
              <span>Game</span>
              <select className="input-antique" value={edition} onChange={(e) => window.location.assign(nostradamusHref(e.target.value))}>
                {editions.data.editions.map((e) => (
                  <option key={e.slug} value={e.slug}>
                    {editionLabel(e, new Date())}
                  </option>
                ))}
              </select>
            </label>
          )}
          {game.kind !== 'ready' ? (
            <Waiting load={game} what="the game" />
          ) : (
            <>
              <JoinBox apiUrl={apiUrl} game={game.data} place={place.kind === 'ready' ? place.data : null} onChange={reload} />
              {place.kind === 'ready' && place.data.kind === 'signed-in' && place.data.mine.status === 'accepted' &&
                game.data.rounds
                  .filter((r) => roundStage(r, new Date()) === 'open')
                  .map((r) => <AnswerForm key={r.id} apiUrl={apiUrl} edition={game.data.slug} round={r} place={place.data} onSaved={reload} />)}
              <Standings game={game.data} />
              {[...game.data.rounds].reverse().map((r) => (
                <RoundCard key={r.id} round={r} place={place.kind === 'ready' ? place.data : null} />
              ))}
            </>
          )}
        </>
      )}
    </>
  );
}

/** myPlace reads the player's place in the game, asking the server only when someone seems signed in. */
function myPlace(apiUrl: string, edition: string): Promise<MyPlace> {
  return seemsSignedIn(browserStorage()) ? fetchMyNostradamus(apiUrl, edition) : Promise.resolve({ kind: 'signed-out' });
}

/** JoinBox tells the player where they stand: sign in, link a TTA username, ask to play, waiting, refused, or in. */
function JoinBox({ apiUrl, game, place, onChange }: { apiUrl: string; game: NostradamusGame; place: MyPlace | null; onChange: () => void }) {
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  if (place === null) return null;
  const state = joinState(place.kind !== 'signed-out', place.kind === 'signed-in', place.kind === 'signed-in' ? place.mine.status : 'none');
  const ask = async () => {
    setBusy(true);
    const r = await askToPlay(apiUrl, game.slug);
    setBusy(false);
    if (r.ok) onChange();
    else setMessage(r.message);
  };
  return (
    <section className="rating-card nostradamus-card" aria-labelledby="nostradamus-join">
      <h2 id="nostradamus-join" className="rating-card-heading">
        Play {game.name}
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
        place.kind === 'signed-in' &&
        game.rounds
          .filter((r) => roundStage(r, new Date()) === 'open')
          .map((r) => {
            const done = answeredOf(r.questions, place.mine.answers);
            return (
              <p key={r.id} className={done.answered < done.of ? 'nostradamus-nudge' : undefined}>
                <strong>{r.name}</strong> is open, {r.closes_at ? timeLeft(r.closes_at, new Date()) : ''}: you have answered {done.answered} of {done.of}.{' '}
                <a href={`#answer-${r.id}`}>{done.answered < done.of ? 'Answer now' : 'Change your answers'}</a>
              </p>
            );
          })}
      {message !== '' && <p role="alert">{message}</p>}
    </section>
  );
}

/** AnswerForm is an open round's questions for an accepted player, filled from their saved answers, with the round's tokens to place. */
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
    <section className="rating-card nostradamus-card" aria-labelledby={`answer-${round.id}`}>
      <h2 id={`answer-${round.id}`} className="rating-card-heading">
        {round.name}: your answers
      </h2>
      <p className="muted">
        Closes {round.closes_at ? `${formatInstant(round.closes_at, zone)} (${timeLeft(round.closes_at, new Date())})` : ''}. Answered{' '}
        {givenFrom(round.questions, draft).length} of {round.questions.length}. Tokens left: {tokensLeft(draft, round.tokens)} of {round.tokens} (±{round.token_points}{' '}
        each).
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
          {round.tokens > 0 && (
            <label className="nostradamus-token">
              <input type="checkbox" checked={draft[q.id]?.token ?? false} onChange={() => setDraft(toggleToken(draft, q.id, round.tokens))} /> Token
            </label>
          )}
        </fieldset>
      ))}
      <p>
        <button type="button" className="btn-antique" disabled={busy} onClick={() => void save()}>
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
    <section className="rating-card nostradamus-card" aria-labelledby="nostradamus-standings">
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

/** RoundCard shows a published round: its questions and deadline while open; once closed, each outcome, the player's own answer, and how many picked each choice. */
function RoundCard({ round, place }: { round: NostradamusRound; place: MyPlace | null }) {
  const stage = roundStage(round, new Date());
  const mine = place?.kind === 'signed-in' ? place.mine.answers : [];
  const zone = viewerZone();
  return (
    <section className="rating-card nostradamus-card" aria-labelledby={`round-${round.id}`}>
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
                {my.token && ' (token)'}
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
