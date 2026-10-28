/** The header's player search, on every page: it asks for the rated players only once the box is used, and leads to a player's page or the leaderboard's search. */
import { useState } from 'react';
import { currentRun, leaderboard } from '@ttac/shared/ratings';
import { DEFAULT_VIEW, modelOf } from '../lib/leaderboard-view';
import { searchHref } from '../lib/player-view';
import SearchBox, { type SearchPlayer } from './SearchBox';
import { useLoad } from './useLoad';

/** HeaderSearch suggests rated players as you type and goes where searchHref says. */
export default function HeaderSearch({ apiUrl }: { apiUrl: string }) {
  const [used, setUsed] = useState(false);
  const board = useLoad(
    apiUrl,
    used ? () => currentRun(apiUrl).then((run) => leaderboard(apiUrl, run.run_id, modelOf(DEFAULT_VIEW), DEFAULT_VIEW.format)) : null,
  );
  const players: SearchPlayer[] = board.kind === 'ready' ? board.data.players : [];
  return (
    <SearchBox
      players={players}
      value=""
      inputClassName="input-antique header-search-input"
      onFocus={() => setUsed(true)}
      onSearch={(term) => term !== '' && window.location.assign(searchHref(players.map((p) => p.name), term))}
    />
  );
}
