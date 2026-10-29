/** The weekly challenge's winners: a finished event, its champions and each week's winner and runner-up. */
import { formatDay } from '@ttac/shared/time';
import { challengeWeeks, type ChallengePlacing } from '@ttac/shared/ratings';
import type { ChallengeTile } from '../lib/challenge-list';
import { challengeHref, challengePicture, champions } from '../lib/challenges';
import { ChallengePlayerLink } from './ChallengeParts';
import { useLoad } from './useLoad';
import Waiting from './Waiting';

/** ChallengeWeeklyPage loads the weeks and lists them. */
export default function ChallengeWeeklyPage({ apiUrl, list }: { apiUrl: string; list: ChallengeTile[] }) {
  const load = useLoad('weeks', () => challengeWeeks(apiUrl));
  return (
    <>
      <div className="analysis-hero-card challenges-hero">
        <div>
          <h1 className="analysis-hero-title">Weekly Challenge winners</h1>
          <p className="analysis-hero-desc">
            From September 2024 to July 2025, a Random Mix challenge was featured every week, and the best verified domination by Sunday 15:00 (European time)
            won. The event has ended; here are its champions, for old times&apos; sake.
          </p>
        </div>
      </div>
      {load.kind !== 'ready' ? (
        <Waiting load={load} what="the weekly winners" />
      ) : (
        <>
          <p className="challenges-champions">
            {champions(load.data.weeks).map(([name, wins]) => (
              <span key={name} className="challenges-chip">
                <ChallengePlayerLink name={name} /> × {wins}
              </span>
            ))}
          </p>
          <section className="rating-card challenges-card" aria-label="Weeks">
            <div className="table-responsive">
              <table className="leaderboard-table">
                <thead>
                  <tr>
                    <th scope="col" className="num">
                      #
                    </th>
                    <th scope="col">Week of</th>
                    <th scope="col">Challenge</th>
                    <th scope="col">Winner</th>
                    <th scope="col">Runner-up</th>
                  </tr>
                </thead>
                <tbody>
                  {load.data.weeks.map((w) => (
                    <tr key={w.week}>
                      <td className="num">{w.week}</td>
                      <td className="muted">{formatDay(w.week_of)}</td>
                      <td>
                        <a className="challenges-inline" href={challengeHref(w.challenge)}>
                          <img src={challengePicture(w.challenge)} alt="" width={34} height={28} loading="lazy" />
                          {list.find((c) => c.id === w.challenge)?.name ?? w.challenge}
                        </a>
                      </td>
                      <td>
                        <Placings list={w.winners} />
                      </td>
                      <td>
                        <Placings list={w.runners_up} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </>
      )}
    </>
  );
}

/** Placings lists names with their dominations, "&" joining a tie. */
function Placings({ list }: { list: ChallengePlacing[] }) {
  if (list.length === 0) return <span className="muted">—</span>;
  return (
    <>
      {list.map((p, i) => (
        <span key={p.player}>
          {i > 0 && (list[i - 1].domination === p.domination ? ' & ' : ', ')}
          <ChallengePlayerLink name={p.player} /> <span className="muted">{p.domination}</span>
        </span>
      ))}
    </>
  );
}
