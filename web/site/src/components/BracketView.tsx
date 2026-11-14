/** A season shown as a bracket: rounds to pick, the late rounds as a tree, a round's groups as a list, and a player's road. */
import { Fragment, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { seasonBracket, type CatalogueSeason, type BracketGroup } from '@ttac/shared/ratings';
import {
  allView,
  bracketGrid,
  bracketPlayers,
  roadsKept,
  compactTree,
  bracketMap,
  bracketRounds,
  anchoredScroll,
  fitZoom,
  groupLabel,
  groupsMatching,
  pinchZoom,
  road,
  seatView,
  seedLabel,
  defaultView,
  stepZoom,
  tapRound,
  shownRounds,
  treeReach,
  type BracketView as View,
  type ViewRound,
} from '../lib/bracket';
import { editionHref } from '@ttac/shared/blitz';
import { seasonName } from '../lib/catalogue-view';
import SearchBox from './SearchBox';
import Waiting from './Waiting';
import Message from './Message';
import { useLoad } from './useLoad';

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
  const load = useLoad(JSON.stringify([run, slug, season]), () => seasonBracket(apiUrl, run, slug, season));
  const [chosen, setChosen] = useState<View | null>(null);
  const [entrant, setEntrant] = useState<number | null>(null);
  const [q, setQ] = useState('');
  const [single, setSingle] = useState<'tree' | 'grid'>('grid');
  useEffect(() => {
    setChosen(null);
    setEntrant(null);
  }, [run, slug, season]);
  if (season === '') return null;

  const rounds = load.kind === 'ready' ? bracketRounds(load.data) : [];
  const view = chosen ?? defaultView(rounds);
  const reach = treeReach(rounds);
  const tree = shownRounds(rounds, view, reach, single);
  const treeable = view.first === view.last && shownRounds(rounds, view, reach, 'tree') !== null;
  const all = allView(rounds);
  const kept = roadsKept(rounds, q);
  const shown = (groups: BracketGroup[], r: number) => (kept === null ? groupsMatching(groups, q) : groups.filter((g) => kept.has(`${r}/${g.number}`)));
  const hint =
    rounds.length < 2
      ? 'Tap a round for its results.'
      : 'Tap a round for its results, or two rounds for the bracket between them.';
  const listed = tree === null ? rounds.slice(view.first, view.last + 1).map((x, k) => ({ x, r: view.first + k })) : [];
  const phone = rounds[view.last];
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
      {load.kind === 'ready' && rounds.length === 0 && (
        <Message form="inline">
          The knockout is not drawn yet. <a href={editionHref(slug, season)}>Swiss rounds: see the standings</a>.
        </Message>
      )}
      {phone !== undefined && (
        <>
          <p className="records-intro bracket-key">
            <span className="bracket-wide-only">{hint} </span>
            Gold: through to the next round. <span className="bracket-to">→ SF 1</span> is where they play next. Choose a name to follow that player.
          </p>
          <div className="bracket-controls">
            <nav aria-label="Rounds" className="bracket-rounds bracket-rounds-wide">
              {rounds.map((x, i) => (
                <Fragment key={x.name}>
                  {i === reach && i > 0 && <span className="bracket-rounds-split" title="The bracket can be drawn from here" aria-hidden="true" />}
                  <button type="button" className="bracket-round" aria-pressed={i >= view.first && i <= view.last} onClick={() => setChosen(tapRound(view, i))}>
                    <span>{x.name}</span>
                    <span className="bracket-round-state">{ROUND_STATE[x.state]}</span>
                  </button>
                </Fragment>
              ))}
              {rounds.length > 1 && (
                <button type="button" className="bracket-round" aria-pressed={view.first === all.first && view.last === all.last} onClick={() => setChosen(all)}>
                  <span>All rounds</span>
                  <span className="bracket-round-state">{rounds.length} rounds</span>
                </button>
              )}
            </nav>
            <nav aria-label="Rounds" className="bracket-rounds bracket-rounds-narrow">
              {rounds.map((x, i) => (
                <button key={x.name} type="button" className="bracket-round" aria-pressed={i === view.last} onClick={() => setChosen({ first: i, last: i, anchor: i })}>
                  <span>{x.name}</span>
                  <span className="bracket-round-state">{ROUND_STATE[x.state]}</span>
                </button>
              ))}
            </nav>
            <div className="bracket-find">
              <label htmlFor="bracket-find">Find a player</label>
              <SearchBox players={bracketPlayers(rounds)} value={q} onChange={setQ} id="bracket-find" inputClassName="bracket-find-input" placeholder="Player name" />
            </div>
          </div>
          {treeable && (
            <div className="bracket-view bracket-wide-only" role="group" aria-label="Show the round">
              <button type="button" className="bracket-zoom-button" aria-pressed={single === 'grid'} onClick={() => setSingle('grid')}>
                Grid
              </button>
              <button type="button" className="bracket-zoom-button" aria-pressed={single === 'tree'} onClick={() => setSingle('tree')}>
                Tree
              </button>
            </div>
          )}
          {entrant !== null && <PlayerRoad rounds={rounds} entrant={entrant} onClose={() => setEntrant(null)} />}
          {tree !== null && <BracketTree rounds={rounds} start={tree.start} end={tree.end} joined={tree.joined} kept={kept} q={q} onPlayer={setEntrant} />}
          {listed.map(({ x, r }) => (
            <div key={x.name} className="bracket-wide-only">
              {listed.length > 1 && <h3 className="bracket-list-head">{x.name}</h3>}
              <div className="bracket-list">
                {shown(x.groups, r).map((g) => (
                  <GroupCard key={g.number} rounds={rounds} r={r} group={g} q={q} onPlayer={setEntrant} />
                ))}
              </div>
            </div>
          ))}
          <div className="bracket-list bracket-list-narrow">
            {shown(phone.groups, view.last).map((g) => (
              <GroupCard key={g.number} rounds={rounds} r={view.last} group={g} q={q} onPlayer={setEntrant} />
            ))}
          </div>
        </>
      )}
    </div>
  );
}

/** BracketTree lays the rounds from start to end out side by side as a tree, each joined round's pairs of groups joined
 * to the group they feed, fitted to the screen until zoomed in or out. */
function BracketTree({
  rounds,
  start,
  end,
  joined,
  kept,
  q,
  onPlayer,
}: {
  rounds: ViewRound[];
  start: number;
  end: number;
  joined: boolean[];
  kept: Set<string> | null;
  q: string;
  onPlayer: (entrant: number) => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const tree = useRef<HTMLDivElement>(null);
  const [chosen, setChosen] = useState<number | null>(null);
  const [fit, setFit] = useState(1);
  const zoom = chosen ?? fit;
  useEffect(() => {
    const b = box.current;
    const t = tree.current;
    if (b === null || t === null) return;
    const measure = () => {
      const was = t.style.zoom;
      t.style.zoom = '1';
      const css = getComputedStyle(b);
      setFit(fitZoom(b.clientWidth - parseFloat(css.paddingLeft) - parseFloat(css.paddingRight), t.scrollWidth));
      t.style.zoom = was;
    };
    const watch = new ResizeObserver(measure);
    watch.observe(b);
    return () => watch.disconnect();
  }, [rounds.length, start, end]);
  // A change of size keeps a point still: between the fingers, or the middle of what is in view.
  const anchor = useRef<{ dx: number; dy: number; from: number; sx: number; sy: number } | null>(null);
  const zoomTo = (next: number | null, x?: number, y?: number) => {
    const b = box.current;
    const t = tree.current;
    if (b !== null && t !== null) {
      const r = t.getBoundingClientRect();
      const bx = b.getBoundingClientRect();
      const ax = x ?? bx.left + b.clientWidth / 2;
      const ay = y ?? (Math.max(r.top, 0) + Math.min(r.bottom, window.innerHeight)) / 2;
      anchor.current = { dx: ax - r.left, dy: ay - r.top, from: zoom, sx: b.scrollLeft, sy: window.scrollY };
    }
    setChosen(next);
  };
  useLayoutEffect(() => {
    if (tree.current !== null) tree.current.style.zoom = String(zoom);
    const a = anchor.current;
    anchor.current = null;
    if (a === null || box.current === null) return;
    box.current.scrollLeft = anchoredScroll(a.sx, a.dx, a.from, zoom);
    window.scrollTo(window.scrollX, anchoredScroll(a.sy, a.dy, a.from, zoom));
  }, [zoom]);
  // Two fingers on the tree zoom it; touch-action in the CSS leaves one finger to scroll.
  const touches = useRef(new Map<number, { x: number; y: number }>());
  const pinch = useRef<{ from: number; apart: number } | null>(null);
  const apart = () => {
    const [a, b] = [...touches.current.values()];
    return a === undefined || b === undefined ? 0 : Math.hypot(a.x - b.x, a.y - b.y);
  };
  const touchDown = (e: React.PointerEvent) => {
    if (e.pointerType !== 'touch') return;
    touches.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (touches.current.size === 2) pinch.current = { from: zoom, apart: apart() };
  };
  const touchMove = (e: React.PointerEvent) => {
    if (!touches.current.has(e.pointerId)) return;
    touches.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    const [a, b] = [...touches.current.values()];
    if (pinch.current !== null && a !== undefined && b !== undefined) {
      zoomTo(pinchZoom(pinch.current.from, pinch.current.apart, apart()), (a.x + b.x) / 2, (a.y + b.y) / 2);
    }
  };
  const touchUp = (e: React.PointerEvent) => {
    touches.current.delete(e.pointerId);
    if (touches.current.size < 2) pinch.current = null;
  };
  const grid = bracketGrid(
    rounds.slice(start, end + 1).map((x) => x.groups),
    joined,
  );
  const squashed = kept === null ? null : compactTree(rounds, start, end, kept, joined);
  // Rows and line ends are set on the elements, as the CSP allows no inline styles.
  useLayoutEffect(() => {
    tree.current?.querySelectorAll<HTMLElement>('[data-span], [data-rows]').forEach((el) => {
      const { span, rows, barTop, barBottom } = el.dataset;
      el.style.gridRow = rows !== undefined ? rows : el.classList.contains('bracket-col') ? `1 / span ${span}` : `span ${span}`;
      if (barTop !== undefined && barBottom !== undefined) {
        el.style.setProperty('--bar-top', `${barTop}%`);
        el.style.setProperty('--bar-bottom', `${barBottom}%`);
      }
    });
  });
  const card = (i: number, g: BracketGroup) => <GroupCard rounds={rounds} r={i} group={g} q={q} onPlayer={onPlayer} titled />;
  const columns = grid.columns.map((c, k) => {
    const i = start + k;
    const x = rounds[i];
    if (x === undefined) return null;
    return (
      <section key={x.name} className="bracket-col" aria-label={x.name} data-span={(squashed?.rows ?? grid.rows) + 1}>
        <h3 className="bracket-col-head">{x.name}</h3>
        {squashed === null
          ? c.items.map((item, j) => (
              <div key={j} data-span={c.span} className={item.length === 2 ? 'bracket-pair bracket-pair-joined' : 'bracket-pair'}>
                {item.map((g) => (
                  <div key={g.number} className="bracket-slot">
                    {card(i, g)}
                  </div>
                ))}
              </div>
            ))
          : squashed.columns[k]?.items.map((it) => {
              const size = it.end - it.start;
              return (
                <div
                  key={it.group.number}
                  data-rows={`${it.start + 2} / ${it.end + 2}`}
                  data-bar-top={it.bar === null ? undefined : ((it.bar[0] - it.start) / size) * 100}
                  data-bar-bottom={it.bar === null ? undefined : 100 - ((it.bar[1] - it.start) / size) * 100}
                  className={['bracket-item', it.out ? 'bracket-item-out' : '', it.bar !== null ? 'bracket-item-in' : ''].join(' ')}
                >
                  <div className="bracket-slot">{card(i, it.group)}</div>
                </div>
              );
            })}
      </section>
    );
  });
  return (
    <div
      className="bracket-tree-box"
      ref={box}
      onPointerDown={touchDown}
      onPointerMove={touchMove}
      onPointerUp={touchUp}
      onPointerCancel={touchUp}
    >
      <div className="bracket-zoom" role="group" aria-label="Zoom the bracket">
        <button type="button" className="bracket-zoom-button" aria-label="Zoom out" onClick={() => zoomTo(stepZoom(zoom, -1))}>
          −
        </button>
        <button type="button" className="bracket-zoom-button" aria-pressed={chosen === null} onClick={() => zoomTo(null)}>
          Fit to screen
        </button>
        <button type="button" className="bracket-zoom-button" aria-label="Zoom in" onClick={() => zoomTo(stepZoom(zoom, 1))}>
          +
        </button>
        <span className="bracket-zoom-size">{Math.round(zoom * 100)}%</span>
      </div>
      <div className="bracket-tree" ref={tree}>
        {columns}
      </div>
    </div>
  );
}

/** GroupCard is one group: its players best first, who went through and where to, each name opening their road. */
function GroupCard({ rounds, r, group, q, onPlayer, titled = false }: { rounds: ViewRound[]; r: number; group: BracketGroup; q: string; onPlayer: (entrant: number) => void; titled?: boolean }) {
  const x = rounds[r];
  if (x === undefined) return null;
  const title = groupLabel(rounds, r, group, titled);
  const state = x.unit === 'Seed' ? 'Qualified' : group.seats.length === 0 ? 'To come' : group.state === 'played' ? 'Finished' : group.state === 'running' ? 'Being played' : 'To play';
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
                {s.name} {seedLabel(s)}
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
            <span className="bracket-road-group">{st.label}</span>
            <ol className="bracket-seats">
              {st.group.seats.map((s) => (
                <li key={s.entrant} className={s.entrant === entrant ? 'bracket-seat bracket-seat-me' : 'bracket-seat'}>
                  <span className="bracket-name">
                    {s.name} {seedLabel(s)}
                  </span>
                </li>
              ))}
            </ol>
            <span className="bracket-road-result">{st.result}</span>
          </li>
        ))}
      </ol>
      <p className="records-intro">
        Route: {steps.map((st) => (st.final ? st.round : `${st.round} ${st.label.toLowerCase()}`)).join(' → ')}.
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
