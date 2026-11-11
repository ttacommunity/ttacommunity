/** One Nostradamus participant's answers, round by round: what they said, their tokens, what happened, and the points. */
import { useEffect, useState } from 'react';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import { nostradamusParticipant, RatingsApiError } from '@ttac/shared/ratings';
import { nameParam } from '../lib/challenges';
import { editionParam, nostradamusHref, pointsText, shownName, valuesText } from '../lib/nostradamus';
import { useLoad } from './useLoad';
import Waiting from './Waiting';

/** NostradamusParticipantPage loads the participant the address names (?edition=, ?name=) and lists their rounds. */
export default function NostradamusParticipantPage({ apiUrl }: { apiUrl: string }) {
  const [asked, setAsked] = useState<{ edition: string | null; name: string } | null>(null);
  useEffect(() => setAsked({ edition: editionParam(window.location.search), name: nameParam(window.location.search) }), []);
  const edition = asked?.edition ?? null;
  const name = asked?.name ?? '';
  const load = useLoad(
    `participant-${edition}-${name}`,
    edition === null || name === ''
      ? null
      : () =>
          nostradamusParticipant(apiUrl, edition, name).catch((e: unknown) => {
            if (e instanceof RatingsApiError && e.status === 404) return null;
            throw e;
          }),
  );

  if (asked !== null && (edition === null || name === '')) return <div className="rating-card waiting-message">Choose a participant from the standings.</div>;
  if (load.kind !== 'ready') return <Waiting load={load} what="this participant" />;
  if (load.data === null) return <div className="rating-card waiting-message">No such participant in this edition.</div>;
  const p = load.data;
  const zone = viewerZone();
  return (
    <>
      <div className="analysis-hero-card">
        <div>
          <h1 className="analysis-hero-title">{shownName(p)}</h1>
          <p className="analysis-hero-desc">
            Their Nostradamus answers in each round that has closed. <a href={nostradamusHref(edition ?? '')}>Back to the standings</a>
          </p>
        </div>
      </div>
      {p.rounds.length === 0 && <div className="rating-card waiting-message">No round has closed yet.</div>}
      {p.rounds.map((r) => (
        <section key={r.round} className="rating-card nostradamus-card" aria-label={r.name}>
          <h2 className="rating-card-heading">
            {r.name}: {r.missed ? `missed (${r.points ?? '–'}, the lowest bot's score)` : r.points === null ? 'not scored yet' : `${r.points} points`}
          </h2>
          <p className="muted">Closed {formatInstant(r.closes_at, zone)}</p>
          {r.answers.length > 0 && (
            <div className="table-responsive">
              <table className="leaderboard-table">
                <caption className="visually-hidden">
                  {r.name}: each question, the answer given, whether a token was on it, what happened, and the points
                </caption>
                <thead>
                  <tr>
                    <th scope="col">Question</th>
                    <th scope="col">Answer</th>
                    <th scope="col">Token</th>
                    <th scope="col">Outcome</th>
                    <th scope="col" className="num">
                      Points
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {r.answers.map((a) => (
                    <tr key={a.question.label}>
                      <td>{a.question.prompt || a.question.label}</td>
                      <td>{valuesText(a.answer)}</td>
                      <td>{a.token ? 'Yes' : <span className="muted">No</span>}</td>
                      <td className={a.question.outcome ? undefined : 'muted'}>{valuesText(a.question.outcome)}</td>
                      <td className="num">{pointsText(a.points)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      ))}
    </>
  );
}
