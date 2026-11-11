/** Nostradamus standings: the edition the address names (else the newest), each participant's points by round. */
import { useEffect, useState } from 'react';
import { nostradamusBoard, nostradamusEditions } from '@ttac/shared/ratings';
import { chosenEdition, editionParam, nostradamusHref, participantHref, places, shownName } from '../lib/nostradamus';
import { useLoad } from './useLoad';
import Waiting from './Waiting';

/** NostradamusBoardPage loads the editions, then the chosen one's standings. */
export default function NostradamusBoardPage({ apiUrl }: { apiUrl: string }) {
  const [wanted, setWanted] = useState<string | null | undefined>(undefined);
  useEffect(() => setWanted(editionParam(window.location.search)), []);
  const editions = useLoad('editions', wanted === undefined ? null : () => nostradamusEditions(apiUrl));
  const edition = editions.kind === 'ready' ? chosenEdition(editions.data.editions, wanted ?? null) : null;
  const board = useLoad(`board-${edition}`, edition === null ? null : () => nostradamusBoard(apiUrl, edition));
  const ranks = board.kind === 'ready' ? places(board.data.standings.map((s) => s.total)) : [];

  return (
    <>
      <div className="analysis-hero-card">
        <div>
          <h1 className="analysis-hero-title">Nostradamus</h1>
          <p className="analysis-hero-desc">
            The community&apos;s prediction game for the World Championship. Each round, participants predict the results and may put tokens on the answers
            they are surest of: a right one wins the token&apos;s points, a wrong one loses them. A round missed scores the lowest of the bots.
          </p>
        </div>
      </div>
      {editions.kind !== 'ready' ? (
        <Waiting load={editions} what="the editions" />
      ) : edition === null ? (
        <div className="rating-card waiting-message">No edition yet.</div>
      ) : (
        <>
          {editions.data.editions.length > 1 && (
            <nav className="nostradamus-editions" aria-label="Editions">
              {editions.data.editions.map((e) => (
                <a key={e.slug} href={nostradamusHref(e.slug)} aria-current={e.slug === edition ? 'page' : undefined}>
                  {e.name}
                </a>
              ))}
            </nav>
          )}
          {board.kind !== 'ready' ? (
            <Waiting load={board} what="the standings" />
          ) : (
            <section className="rating-card nostradamus-card" aria-label={board.data.name}>
              <h2 className="rating-card-heading">{board.data.name}</h2>
              {board.data.standings.length === 0 ? (
                <p className="muted">No round has been scored yet.</p>
              ) : (
                <div className="table-responsive">
                  <table className="leaderboard-table">
                    <caption className="visually-hidden">{board.data.name}: each participant&apos;s points in each round scored, and their total</caption>
                    <thead>
                      <tr>
                        <th scope="col" className="num">
                          #
                        </th>
                        <th scope="col">Participant</th>
                        {board.data.rounds.map((r) => (
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
                      {board.data.standings.map((s, i) => {
                        return (
                          <tr key={s.name}>
                            <td className="num">{ranks[i]}</td>
                            <td>
                              <a href={participantHref(board.data.slug, s.name)}>{shownName(s)}</a>
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
                        );
                      })}
                    </tbody>
                  </table>
                  <p className="muted">A greyed score is a round missed, scored as the lowest of the bots.</p>
                </div>
              )}
            </section>
          )}
        </>
      )}
    </>
  );
}
