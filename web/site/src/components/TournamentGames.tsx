/** A tournament's games, newest first, narrowed by season, stage and group, with one-click replay codes. */
import { useEffect, useRef, useState } from 'react';
import { catalogueGames, type CatalogueGames, type CatalogueSeason, type GamesFilter } from '@ttac/shared/ratings';
import { catalogueGameRow, plural, seasonName, stageName } from '../lib/catalogue-view';
import { gamesTag } from '../lib/games-table';
import GamesTable, { GamesPages } from './GamesTable';
import PlayerPicker from './PlayerPicker';
import Waiting from './Waiting';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: CatalogueGames };

/** TournamentGames lists one tournament's games for a run, with filters that follow its seasons and stages; pick chooses a season from outside. */
export default function TournamentGames({
  apiUrl,
  run,
  slug,
  seasons,
  players,
  pick,
  seasonWord,
  stageWord,
}: {
  apiUrl: string;
  run: number;
  slug: string;
  seasons: CatalogueSeason[];
  players: string[];
  pick: { season: string; n: number };
  /** What the tournament calls a season and a stage. */
  seasonWord: string;
  stageWord: string;
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
        {data !== null && <span className="panel-tag">{gamesTag(data.total, data.coded, filter.coded === true)}</span>}
      </div>
      <div className="games-filters">
        <label>
          <span>{seasonWord}</span>
          <select className="input-antique" id="games-season" value={filter.season ?? ''} onChange={(e) => narrow({ season: e.target.value, stage: '', group: '' })}>
            <option value="">All {plural(seasonWord).toLowerCase()}</option>
            {seasons.map((s) => (
              <option key={s.label} value={s.label}>
                {seasonName(s.label, seasonWord)}
              </option>
            ))}
          </select>
        </label>
        {stages.length > 0 && (
          <label>
            <span>{stageWord}</span>
            <select className="input-antique" id="games-stage" value={filter.stage ?? ''} onChange={(e) => narrow({ stage: e.target.value, group: '' })}>
              <option value="">All {plural(stageWord).toLowerCase()}</option>
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
            <select className="input-antique" id="games-group" value={filter.group ?? ''} onChange={(e) => narrow({ group: e.target.value })}>
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
        <label className="games-coded check-inline">
          <input id="games-coded" type="checkbox" checked={filter.coded === true} onChange={(e) => narrow({ coded: e.target.checked })} />
          <span>Only games with a replay code</span>
        </label>
      </div>
      <p className="records-intro">
        Replay codes come from CGE's tournament pages, so not every game has one: older games and those we have not matched to CGE yet show none. Click a code to
        copy it.
      </p>
      <GamesTable
        caption="The games these filters keep, newest first"
        context={seasonWord}
        rows={data?.games.map((g) => catalogueGameRow(g, seasonWord)) ?? []}
        waiting={data === null ? <Waiting load={load} what="the games" form="row" colSpan={5} /> : undefined}
        empty="No games match these filters."
      />
      {data !== null && <GamesPages page={data.page} pages={data.pages} onPage={(page) => narrow({ page })} />}
    </div>
  );
}
