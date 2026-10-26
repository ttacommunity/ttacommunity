/** The Challenges section: the game's challenges by set and group, each with its picture and verified record. */
import { useEffect, useState } from 'react';
import { challenges, type Challenge, type ChallengeList } from '@ttac/shared/ratings';
import { challengePicture, challengeSets } from '../lib/challenges';
import Waiting from './Waiting';
import './Challenges.css';

type Load<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

/** ChallengesPage loads the challenges and lists them by set. */
export default function ChallengesPage({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load<ChallengeList>>({ kind: 'loading' });
  useEffect(() => {
    challenges(apiUrl)
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
  }, [apiUrl]);
  return (
    <>
      <div className="tournaments-header">
        <h1 className="tournaments-title">
          <span>🏆</span> Challenges
        </h1>
        <div className="tournaments-subtitle">
          The game&apos;s single-player challenges and the highest domination entered for each, verified by a screenshot in
          the Through the Ages Discord.
        </div>
      </div>
      {load.kind !== 'ready' ? (
        <Waiting load={load} what="the challenges" />
      ) : (
        challengeSets(load.data.challenges).map((set) => (
          <section key={set.id} className="rating-card challenges-card" aria-labelledby={`challenges-${set.id}`}>
            <h2 id={`challenges-${set.id}`} className="rating-card-heading">
              {set.label}
            </h2>
            {set.groups.map((group) => (
              <div key={group.id} className="challenges-table-wrap">
                <table className="challenges-table">
                  <caption>{group.label}</caption>
                  <thead>
                    <tr>
                      <th scope="col">
                        <span className="visually-hidden">Picture</span>
                      </th>
                      <th scope="col">Challenge</th>
                      <th scope="col" className="num">
                        Difficulty
                      </th>
                      <th scope="col" className="num">
                        Record
                      </th>
                      <th scope="col">Held by</th>
                    </tr>
                  </thead>
                  <tbody>
                    {group.challenges.map((c) => (
                      <ChallengeRow key={c.id} challenge={c} />
                    ))}
                  </tbody>
                </table>
              </div>
            ))}
          </section>
        ))
      )}
    </>
  );
}

/** ChallengeRow is one challenge: its picture, name, difficulty, and record with the proof it links. */
function ChallengeRow({ challenge: c }: { challenge: Challenge }) {
  return (
    <tr>
      <td className="challenges-picture">
        <img src={challengePicture(c.id)} alt="" width={68} height={57} loading="lazy" />
      </td>
      <th scope="row">{c.name}</th>
      <td className="num">{c.difficulty}</td>
      <td className="num">
        {c.record ? (
          <a href={c.record.proof_url} target="_blank" rel="noopener noreferrer" title="The proof, in the TtA Discord">
            {c.record.domination}
          </a>
        ) : (
          '–'
        )}
      </td>
      <td>{c.record ? c.record.player : <span className="muted">None yet</span>}</td>
    </tr>
  );
}
