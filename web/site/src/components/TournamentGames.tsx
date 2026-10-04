/** A tournament's games, newest first, narrowed by season, stage and group, with one-click replay codes. */
import { useEffect, useId, useRef, useState } from 'react';
import { catalogueGames, type CatalogueGames, type CatalogueSeason, type GamesFilter } from '@ttac/shared/ratings';
import { playerHref } from '../lib/player-view';
import { gameStateLabel, gameWhere, outcomeNote, seasonName, stageName } from '../lib/catalogue-view';
import { suggest } from '../lib/leaderboard-view';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: CatalogueGames };

const count = (n: number) => n.toLocaleString('en-US');
const score = (n: number) => (Number.isInteger(n) ? String(n) : n.toFixed(1));

/** TournamentGames lists one tournament's games for a run, with filters that follow its seasons and stages; pick chooses a season from outside. */
export default function TournamentGames({
  apiUrl,
  run,
  slug,
  seasons,
  players,
  pick,
}: {
  apiUrl: string;
  run: number;
  slug: string;
  seasons: CatalogueSeason[];
  players: string[];
  pick: { season: string; n: number };
}) {
  const [filter, setFilter] = useState<GamesFilter>({ page: 1 });
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (pick.n === 0) return;
    setFilter({ season: pick.season, page: 1 });
    panel.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }, [pick]);
  const [load, setLoad] = useState<Load>({ kind: 'loading' });
  useEffect(() => {
    let live = true;
    setLoad({ kind: 'loading' });
    catalogueGames(apiUrl, run, slug, filter)
      .then((data) => live && setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => live && setLoad({ kind: 'error', message: e instanceof Error ? e.message : 'The games could not be loaded.' }));
    return () => {
      live = false;
    };
  }, [apiUrl, run, slug, filter]);

  const season = seasons.find((s) => s.label === filter.season);
  const stages = season !== undefined && (season.stages.length > 1 || season.stages[0]?.label !== '') ? season.stages : [];
  const data = load.kind === 'ready' ? load.data : null;
  const narrow = (next: Partial<GamesFilter>) => setFilter((f) => ({ ...f, ...next, page: next.page ?? 1 }));

  return (
    <div className="panel-card games-panel" ref={panel}>
      <div className="panel-header panel-header-split">
        <h2 className="panel-header-title">Games</h2>
        {data !== null && (
          <span className="panel-tag">
            {count(data.total)} games{filter.coded ? ' with a replay code' : ` · ${count(data.coded)} with a replay code`}
          </span>
        )}
      </div>
      <div className="games-filters">
        <label>
          <span>Season</span>
          <select id="games-season" value={filter.season ?? ''} onChange={(e) => narrow({ season: e.target.value, stage: '', group: '' })}>
            <option value="">All seasons</option>
            {seasons.map((s) => (
              <option key={s.label} value={s.label}>
                {seasonName(s.label)}
              </option>
            ))}
          </select>
        </label>
        {stages.length > 0 && (
          <label>
            <span>Stage</span>
            <select id="games-stage" value={filter.stage ?? ''} onChange={(e) => narrow({ stage: e.target.value, group: '' })}>
              <option value="">All stages</option>
              {stages.map((st) => (
                <option key={st.label} value={st.label}>
                  {stageName(st.label)}
                </option>
              ))}
            </select>
          </label>
        )}
        {data !== null && data.groups.length > 0 && (
          <label>
            <span>Group</span>
            <select id="games-group" value={filter.group ?? ''} onChange={(e) => narrow({ group: e.target.value })}>
              <option value="">All groups</option>
              {data.groups.map((g) => (
                <option key={g} value={g}>
                  {g}
                </option>
              ))}
            </select>
          </label>
        )}
        <PlayerPicker players={players} value={filter.player ?? ''} onPick={(player) => narrow({ player })} />
        <label className="games-coded">
          <input id="games-coded" type="checkbox" checked={filter.coded === true} onChange={(e) => narrow({ coded: e.target.checked })} />
          <span>Only games with a replay code</span>
        </label>
      </div>
      <p className="records-intro">
        Replay codes come from CGE's tournament pages, so not every game has one: older games and those we have not matched to CGE yet show none. Click a code to
        copy it.
      </p>
      <div className="trophy-table-container games-table">
        <table className="trophy-table">
          <caption className="records-intro">The games these filters keep, newest first</caption>
          <thead>
            <tr>
              <th className="col-center">Date</th>
              <th>Season</th>
              <th>Game</th>
              <th>Players &amp; scores</th>
              <th>Replay code</th>
            </tr>
          </thead>
          <tbody>
            {load.kind === 'loading' && (
              <tr>
                <td colSpan={5} className="cell-muted">
                  Loading games…
                </td>
              </tr>
            )}
            {load.kind === 'error' && (
              <tr>
                <td colSpan={5} className="cell-muted">
                  {load.message}
                </td>
              </tr>
            )}
            {data !== null && data.games.length === 0 && (
              <tr>
                <td colSpan={5} className="cell-muted">
                  No games match these filters.
                </td>
              </tr>
            )}
            {data?.games.map((g) => (
              <tr key={g.match_id} className={g.state !== 'played' ? 'game-never-played' : undefined}>
                <td className="cell-date">{g.date}</td>
                <td className="cell-muted">{seasonName(g.season)}</td>
                <td>
                  {gameWhere(g)}
                  {gameStateLabel(g.state, g.rated) !== '' && <span className="panel-tag game-state">{gameStateLabel(g.state, g.rated)}</span>}
                </td>
                <td>
                  <div className="game-seats">
                    {g.seats.map((s, i) => (
                      <span key={`${s.name}-${i}`} className="game-seat">
                        <a href={playerHref(s.name)}>{s.name}</a> {score(s.score)}
                        {outcomeNote(s.outcome) !== '' && <span className="game-outcome"> ({outcomeNote(s.outcome)})</span>}
                      </span>
                    ))}
                  </div>
                </td>
                <td>{g.replay_code === null ? <span className="cell-muted">—</span> : <ReplayCode code={g.replay_code} />}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {data !== null && data.pages > 1 && (
        <div className="games-pages">
          <button type="button" className="btn-antique" disabled={data.page <= 1} onClick={() => narrow({ page: data.page - 1 })}>
            ‹ Newer
          </button>
          <span>
            Page {count(data.page)} of {count(data.pages)}
          </span>
          <button type="button" className="btn-antique" disabled={data.page >= data.pages} onClick={() => narrow({ page: data.page + 1 })}>
            Older ›
          </button>
        </div>
      )}
    </div>
  );
}

/** PlayerPicker narrows the games to one player, suggesting the tournament's players as you type; an exact name or a suggestion picks. */
function PlayerPicker({ players, value, onPick }: { players: string[]; value: string; onPick: (player: string) => void }) {
  const [text, setText] = useState(value);
  const listId = useId();
  useEffect(() => setText(value), [value]);
  const found = suggest(
    players.map((name) => ({ name })),
    text,
  );
  const choose = (typed: string) => {
    const exact = players.find((p) => p.toLowerCase() === typed.trim().toLowerCase());
    if (exact !== undefined) {
      setText(exact);
      onPick(exact);
    }
  };
  return (
    <label className="games-player">
      <span>Player</span>
      <span className="games-player-field">
        <input
          id="games-player"
          type="search"
          list={listId}
          value={text}
          placeholder="Search a name"
          autoComplete="off"
          onChange={(e) => {
            setText(e.target.value);
            if (players.some((p) => p === e.target.value)) choose(e.target.value);
            if (e.target.value === '' && value !== '') onPick('');
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              choose(text);
            }
          }}
        />
        {value !== '' && (
          <button type="button" className="games-player-clear" onClick={() => onPick('')} aria-label="Show every player's games">
            ✕
          </button>
        )}
      </span>
      <datalist id={listId}>
        {found.map((p) => (
          <option key={p.name} value={p.name} />
        ))}
      </datalist>
    </label>
  );
}

/** ReplayCode is a replay code that copies itself when clicked. */
function ReplayCode({ code }: { code: string }) {
  const [note, setNote] = useState('');
  const copy = () => {
    const done = (text: string) => {
      setNote(text);
      setTimeout(() => setNote(''), 1500);
    };
    if (!navigator.clipboard) return done('Select to copy');
    navigator.clipboard.writeText(code).then(
      () => done('Copied'),
      () => done('Select to copy'),
    );
  };
  return (
    <button type="button" className="game-code" onClick={copy} title="Copy the replay code" aria-label={`Copy replay code ${code}`}>
      <code>{code}</code>
      <span className="game-code-note" aria-live="polite">
        {note}
      </span>
    </button>
  );
}
