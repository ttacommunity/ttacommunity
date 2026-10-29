/** The challenges' highscores: every challenge's record by set as cards or a compact grid, the totals of each scope, and the most records held. */
import { useState } from 'react';
import Segmented from '@ttac/shared/Segmented';
import { challengeEntries, challengeHighscores, type Challenge, type ChallengeScope } from '@ttac/shared/ratings';
import { downloadName, downloadRows, toCsv } from '../lib/challenge-download';
import { challengeHref, challengePicture, challengeSets, formatMeasure, scopeLeaders, type Measure, type SetId } from '../lib/challenges';
import { ChallengePlayerLink, Difficulty, MeasureSwitch, ProofMark, SetGrid, useChoice, VerifiedSwitch } from './ChallengeParts';
import { useLoad } from './useLoad';
import Waiting from './Waiting';

const SETS: { value: SetId; label: string }[] = [
  { value: 'original', label: 'Original' },
  { value: 'expansion', label: 'Expansion' },
  { value: 'mix', label: 'Random Mix' },
];

/** ChallengesHighscores loads the highscores for the chosen entries and shows them. */
export default function ChallengesHighscores({ apiUrl }: { apiUrl: string }) {
  const [verified, setVerified] = useChoice('verified');
  const [measure, setMeasure] = useChoice('measure');
  const [view, setView] = useChoice('view');
  const [set, setSet] = useState<SetId>('mix');
  const load = useLoad(`highscores-${verified}`, () => challengeHighscores(apiUrl, verified === 'verified'));

  return (
    <>
      <div className="analysis-hero-card challenges-hero">
        <div>
          <h1 className="analysis-hero-title">Domination Highscores</h1>
          <p className="analysis-hero-desc">
            Domination is your final score minus the score of the second-place player.{' '}
            {verified === 'verified' ? 'Official records: only entries with a linked Discord proof count.' : 'All entries, including ones without proof.'}{' '}
            <a href="/challenges/submit/">Submit your domination →</a>
          </p>
        </div>
      </div>

      <div className="challenges-bar">
        <div className="challenges-bar-group">
          <Segmented label="Set" look="raised" value={set} onChange={setSet} options={SETS} />
          <Segmented
            label="View"
            look="raised"
            value={view}
            onChange={setView}
            options={[
              { value: 'cards', label: 'Cards' },
              { value: 'compact', label: 'Compact' },
            ]}
          />
        </div>
        <div className="challenges-bar-group">
          <MeasureSwitch value={measure} onChange={setMeasure} />
          <VerifiedSwitch value={verified} onChange={setVerified} />
        </div>
      </div>

      {load.kind !== 'ready' ? (
        <Waiting load={load} what="the highscores" />
      ) : (
        <>
          <Highscores challenges={load.data.challenges} scopes={load.data.scopes} holders={load.data.record_holders} set={set} measure={measure} view={view} />
          <Download apiUrl={apiUrl} challenges={load.data.challenges} />
        </>
      )}
    </>
  );
}

interface Props {
  challenges: Challenge[];
  scopes: ChallengeScope[];
  holders: { player: string; records: number }[];
  set: SetId;
  measure: Measure;
  view: 'cards' | 'compact';
}

function Highscores({ challenges, scopes, holders, set, measure, view }: Props) {
  const scopeOf = (s: string, group = '') => scopes.find((x) => x.set === s && x.group === group);
  const groups = challengeSets(challenges).find((s) => s.id === set)?.groups ?? [];
  const setScope = scopeOf(set);
  const totals = [...SETS.map((s) => ({ label: s.label, scope: scopeOf(s.value) })), { label: 'Everything', scope: scopeOf('all') }];

  return (
    <>
      {setScope && (
        <p className="challenges-set-totals">
          <TotalsLine scope={setScope} measure={measure} />
        </p>
      )}

      {view === 'compact' ? (
        <section className="rating-card challenges-card">
          <SetGrid
            groups={groups}
            label={(_cs, group) => {
              const scope = scopeOf(set, group);
              const best = scope && scopeLeaders(scope, measure)[0];
              return (
                scope && (
                  <>
                    <div className="muted">Pooled {scope.pooled}</div>
                    {best && (
                      <div className="muted">
                        Best {formatMeasure(measure === 'rating' ? best.rating : best.total, measure)} {best.player}
                      </div>
                    )}
                  </>
                )
              );
            }}
            cell={(c) => (
              <>
                <Holder challenge={c} />
                <div className="muted">
                  {c.players}P · {c.entrants} players
                </div>
              </>
            )}
          />
        </section>
      ) : (
        groups.map((g) => {
          const scope = scopeOf(set, g.id);
          return (
            <section key={g.id} className="rating-card challenges-group" data-group={g.id} aria-labelledby={`group-${g.id}`}>
              <div className="challenges-group-head">
                <h2 id={`group-${g.id}`} className="rating-card-heading">
                  {g.label}
                </h2>
                {scope && <TotalsLine scope={scope} measure={measure} />}
              </div>
              <div className="challenges-cards">
                {g.challenges.map((c) => (
                  <div key={c.id} className="challenges-tile">
                    <a href={challengeHref(c.id)} aria-label={c.name}>
                      <img className="challenges-tile-img" src={challengePicture(c.id)} alt="" loading="lazy" width={340} height={284} />
                    </a>
                    <div className="challenges-tile-body">
                      <a className="challenges-tile-title" href={challengeHref(c.id)}>
                        {c.name}
                      </a>
                      <div className="challenges-tile-meta">
                        {c.players}P <Difficulty value={c.difficulty} /> <span className="muted">{c.entrants} players</span>
                      </div>
                      <div className="challenges-tile-record">
                        <Holder challenge={c} />
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          );
        })
      )}

      <section className="rating-card challenges-card" aria-labelledby="challenges-totals">
        <h2 id="challenges-totals" className="rating-card-heading">
          Totals
        </h2>
        <p className="challenges-lead">
          <strong>Pooled</strong> adds up the record of every challenge, as if all players formed one team. <strong>Best single player</strong> ranks players
          by{' '}
          {measure === 'rating' ? 'rating: each best as a percentage of the record, averaged over the challenges in scope.' : 'the sum of their own best dominations.'}
        </p>
        <div className="table-responsive">
          <table className="leaderboard-table">
            <thead>
              <tr>
                <th scope="col">Scope</th>
                <th scope="col" className="num">
                  Pooled
                </th>
                <th scope="col" className="num">
                  Records set
                </th>
                <th scope="col">Best single player</th>
                <th scope="col">Runners-up</th>
              </tr>
            </thead>
            <tbody>
              {totals.map(
                ({ label, scope }) =>
                  scope && (
                    <tr key={label}>
                      <th scope="row">{label}</th>
                      <td className="num">{scope.pooled}</td>
                      <td className="num">
                        {scope.covered}/{scope.size}
                      </td>
                      <td>
                        <Leader scope={scope} measure={measure} />
                      </td>
                      <td>
                        {scopeLeaders(scope, measure)
                          .slice(1)
                          .map((p, i) => (
                            <span key={p.player}>
                              {i > 0 && ', '}
                              <ChallengePlayerLink name={p.player} />{' '}
                              <span className="muted">{formatMeasure(measure === 'rating' ? p.rating : p.total, measure)}</span>
                            </span>
                          ))}
                      </td>
                    </tr>
                  ),
              )}
            </tbody>
          </table>
        </div>
      </section>

      <section className="rating-card challenges-card" aria-labelledby="challenges-holders">
        <h2 id="challenges-holders" className="rating-card-heading">
          Most records held
        </h2>
        {holders.length === 0 ? (
          <p className="muted">No records yet.</p>
        ) : (
          <ol className="challenges-podium">
            {holders.map((h) => (
              <li key={h.player}>
                <ChallengePlayerLink name={h.player} /> <span className="muted">— {h.records} records</span>
              </li>
            ))}
          </ol>
        )}
      </section>
    </>
  );
}

/** Holder is a challenge's record and who holds it. */
function Holder({ challenge: c }: { challenge: Challenge }) {
  if (c.record === null) return <span className="muted">No entry yet</span>;
  return (
    <span className="challenges-holder">
      <strong className="challenges-score">{c.record.domination}</strong> <ChallengePlayerLink name={c.record.player} /> <ProofMark url={c.record.proof_url} />
    </span>
  );
}

/** Leader is a scope's best player and how many of its challenges they played. */
function Leader({ scope, measure }: { scope: ChallengeScope; measure: Measure }) {
  const best = scopeLeaders(scope, measure)[0];
  if (best === undefined) return <span className="muted">—</span>;
  return (
    <>
      <strong>{formatMeasure(measure === 'rating' ? best.rating : best.total, measure)}</strong> <ChallengePlayerLink name={best.player} />{' '}
      <span className="muted">
        ({best.played}/{scope.size} played)
      </span>
    </>
  );
}

/** TotalsLine is a scope's pooled record and best player in one line. */
function TotalsLine({ scope, measure }: { scope: ChallengeScope; measure: Measure }) {
  const best = scopeLeaders(scope, measure)[0];
  return (
    <span className="challenges-totals-line">
      <span title="Every record here added up: what all players together have achieved">
        Pooled <strong>{scope.pooled}</strong>
        {scope.covered < scope.size && (
          <span className="muted">
            {' '}
            ({scope.covered}/{scope.size})
          </span>
        )}
      </span>
      {best && (
        <span title={measure === 'rating' ? 'Highest rating here' : "Highest sum of one player's own bests here"}>
          Best player <strong>{formatMeasure(measure === 'rating' ? best.rating : best.total, measure)}</strong> <ChallengePlayerLink name={best.player} />
        </span>
      )}
    </span>
  );
}

/** Download offers every entry as a file: CSV for spreadsheets, or JSON. */
function Download({ apiUrl, challenges }: { apiUrl: string; challenges: Challenge[] }) {
  const [failed, setFailed] = useState<string | null>(null);
  const save = async (ext: 'csv' | 'json') => {
    setFailed(null);
    try {
      const rows = downloadRows((await challengeEntries(apiUrl)).entries, challenges);
      const blob =
        ext === 'csv' ? new Blob([toCsv(rows)], { type: 'text/csv;charset=utf-8' }) : new Blob([JSON.stringify(rows, null, 1)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = downloadName(new Date(), ext);
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e: unknown) {
      setFailed(e instanceof Error ? e.message : String(e));
    }
  };
  return (
    <p className="challenges-download">
      Download all data:{' '}
      <button type="button" className="btn-antique" onClick={() => void save('csv')}>
        CSV (Excel)
      </button>{' '}
      <button type="button" className="btn-antique" onClick={() => void save('json')}>
        JSON
      </button>
      {failed !== null && (
        <span className="challenges-download-failed" role="alert">
          {' '}
          Could not download: {failed}
        </span>
      )}
    </p>
  );
}
