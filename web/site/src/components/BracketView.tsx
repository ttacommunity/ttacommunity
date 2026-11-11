/** A season shown as a bracket: rounds to pick, the late rounds as a tree, a round's groups as a list, and a player's road. */
import { useEffect, useRef, useState } from 'react';
import { seasonBracket, type CatalogueSeason, type BracketGroup, type SeasonBracket } from '@ttac/shared/ratings';
import type { Load } from '@ttac/shared/load';
import { defaultRound, bracketMap, bracketRounds, groupsMatching, road, seatView, treeStart, type ViewRound } from '../lib/bracket';
import { seasonName } from '../lib/catalogue-view';
import Waiting from './Waiting';

const ROUND_STATE = { finished: 'Finished', in_play: 'Being played', to_come: 'To come' };

/** BracketView shows one season of a bracket tournament, with a picker for its other seasons with games. */
export default function BracketView({
  apiUrl,
  run,
  slug,
  seasons,
  seasonWord,
  season,
  onSeason,
}: {
  apiUrl: string;
  run: number;
  slug: string;
  seasons: CatalogueSeason[];
  seasonWord: string;
  season: string;
  onSeason: (season: string) => void;
}) {
  const played = seasons.filter((s) => s.games > 0);
  const [load, setLoad] = useState<Load<SeasonBracket>>({ kind: 'loading' });
  const [round, setRound] = useState<number | null>(null);
  const [entrant, setEntrant] = useState<number | null>(null);
  const [q, setQ] = useState('');
  useEffect(() => {
    let live = true;
    setLoad({ kind: 'loading' });
    setRound(null);
    setEntrant(null);
    seasonBracket(apiUrl, run, slug, season)
      .then((data) => live && setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => live && setLoad({ kind: 'error', message: e instanceof Error ? e.message : 'The bracket could not be loaded.' }));
    return () => {
      live = false;
    };
  }, [apiUrl, run, slug, season]);
  if (season === '') return null;

  const rounds = load.kind === 'ready' ? bracketRounds(load.data) : [];
  const start = treeStart(rounds);
  const r = round ?? defaultRound(rounds);
  const inTree = r >= start;
  const shown = rounds[r];
  return (
    <div className="panel-card bracket-panel">
      <div className="panel-header panel-header-split">
        <h2 className="panel-header-title">Bracket</h2>
        {played.length > 1 ? (
          <label className="bracket-season">
            <span className="visually-hidden">{seasonWord}</span>
            <select id="bracket-season" className="input-antique" value={season} onChange={(e) => onSeason(e.target.value)}>
              {[...played].reverse().map((x) => (
                <option key={x.label} value={x.label}>
                  {seasonName(x.label, seasonWord)}
                </option>
              ))}
            </select>
          </label>
        ) : (
          <span className="panel-tag">{seasonName(season, seasonWord)}</span>
        )}
      </div>
      {load.kind !== 'ready' && <Waiting load={load} what="the bracket" />}
      {shown !== undefined && (
        <>
          <div className="bracket-controls">
            <nav aria-label="Rounds" className="bracket-rounds">
              {rounds.map((x, i) => (
                <button key={x.name} type="button" className="bracket-round" aria-pressed={i === r} onClick={() => setRound(i)}>
                  <span>{x.name}</span>
                  <span className="bracket-round-state">{ROUND_STATE[x.state]}</span>
                </button>
              ))}
            </nav>
            <label className="bracket-find">
              Find a player
              <input type="search" value={q} onChange={(e) => setQ(e.target.value)} />
            </label>
          </div>
          <p className="records-intro bracket-key">
            Gold: through to the next round. <span className="bracket-to">→ SF 1</span> is where they play next. Choose a name to follow that player.
          </p>
          {entrant !== null && <PlayerRoad rounds={rounds} entrant={entrant} onClose={() => setEntrant(null)} />}
          {inTree && <BracketTree rounds={rounds} start={start} q={q} onPlayer={setEntrant} />}
          <div className={inTree ? 'bracket-list bracket-list-narrow' : 'bracket-list'}>
            {groupsMatching(shown.groups, q).map((g) => (
              <GroupCard key={g.number} rounds={rounds} r={r} group={g} q={q} onPlayer={setEntrant} />
            ))}
          </div>
        </>
      )}
    </div>
  );
}

/** BracketTree lays the rounds from start to the final out as a tree, each pair of groups joined to the group it feeds. */
function BracketTree({ rounds, start, q, onPlayer }: { rounds: ViewRound[]; start: number; q: string; onPlayer: (entrant: number) => void }) {
  return (
    <div className="bracket-tree-box">
      <div className="bracket-tree">
        {rounds.slice(start).map((x, k) => {
          const i = start + k;
          const pairs = x.groups.length === 1 ? [x.groups] : x.groups.flatMap((g, j) => (j % 2 === 0 ? [x.groups.slice(j, j + 2)] : []));
          return (
            <section key={x.name} className="bracket-col" aria-label={x.name}>
              <h3 className="bracket-col-head">{x.name}</h3>
              <div className="bracket-col-body">
                {pairs.map((pair, j) => (
                  <div key={j} className={pair.length === 2 ? 'bracket-pair bracket-pair-joined' : 'bracket-pair'}>
                    {pair.map((g) => (
                      <GroupCard key={g.number} rounds={rounds} r={i} group={g} q={q} onPlayer={onPlayer} titled />
                    ))}
                  </div>
                ))}
              </div>
            </section>
          );
        })}
      </div>
    </div>
  );
}

/** GroupCard is one group: its players best first, who went through and where to, each name opening their road. */
function GroupCard({ rounds, r, group, q, onPlayer, titled = false }: { rounds: ViewRound[]; r: number; group: BracketGroup; q: string; onPlayer: (entrant: number) => void; titled?: boolean }) {
  const x = rounds[r];
  if (x === undefined) return null;
  const title = x.groups.length === 1 ? x.name : titled ? `${x.short} · Group ${group.number}` : `Group ${group.number}`;
  const state = group.seats.length === 0 ? 'To come' : group.state === 'played' ? 'Finished' : group.state === 'running' ? 'Being played' : 'To play';
  const want = q.trim().toLowerCase();
  return (
    <article className="bracket-card">
      <h4 className="bracket-card-head">
        <span>{title}</span>
        <span className="bracket-card-state">{state}</span>
      </h4>
      {group.seats.length === 0 && <p className="bracket-card-empty">Players to come</p>}
      <ol className="bracket-seats">
        {group.seats.map((s) => {
          const v = seatView(rounds, r, group, s);
          const hit = want !== '' && s.name.toLowerCase().includes(want);
          return (
            <li key={s.entrant} className={['bracket-seat', v.through ? 'bracket-seat-through' : '', v.out ? 'bracket-seat-out' : '', hit ? 'bracket-seat-hit' : ''].join(' ')}>
              <button type="button" className="bracket-name" onClick={() => onPlayer(s.entrant)}>
                {s.name}
              </button>
              <span className="bracket-points">{v.points}</span>
              <span className="bracket-to">{v.to}</span>
            </li>
          );
        })}
      </ol>
    </article>
  );
}

/** PlayerRoad follows one player through the bracket: each of their groups, then where those sit among every group. */
function PlayerRoad({ rounds, entrant, onClose }: { rounds: ViewRound[]; entrant: number; onClose: () => void }) {
  const steps = road(rounds, entrant);
  const map = bracketMap(rounds, entrant);
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    heading.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    heading.current?.focus();
  }, [entrant]);
  const name = steps[0]?.seat.name ?? '';
  return (
    <section className="bracket-road" aria-labelledby="bracket-road-heading">
      <div className="bracket-road-top">
        <h3 id="bracket-road-heading" ref={heading} tabIndex={-1}>
          {name}'s road
        </h3>
        <button type="button" className="bracket-road-close" onClick={onClose}>
          Close
        </button>
      </div>
      <ol className="bracket-road-steps">
        {steps.map((st) => (
          <li key={st.round} className="bracket-road-step">
            <span className="bracket-road-round">{st.round}</span>
            <span className="bracket-road-group">{st.final ? 'The final group' : `Group ${st.group.number}`}</span>
            <ol className="bracket-seats">
              {st.group.seats.map((s) => (
                <li key={s.entrant} className={s.entrant === entrant ? 'bracket-seat bracket-seat-me' : 'bracket-seat'}>
                  <span className="bracket-name">{s.name}</span>
                </li>
              ))}
            </ol>
            <span className="bracket-road-result">{st.result}</span>
          </li>
        ))}
      </ol>
      <p className="records-intro">
        Route: {steps.map((st) => (st.final ? st.round : `${st.round} group ${st.group.number}`)).join(' → ')}.
      </p>
      <div className="bracket-map" aria-hidden="true">
        {map.map((m) => (
          <div key={m.name} className="bracket-map-row">
            <span className="bracket-map-name">{m.name}</span>
            <span className="bracket-map-cells">
              {m.cells.map((on, i) => (
                <span key={i} className={on ? 'bracket-map-cell bracket-map-cell-on' : 'bracket-map-cell'} />
              ))}
            </span>
          </div>
        ))}
      </div>
    </section>
  );
}
