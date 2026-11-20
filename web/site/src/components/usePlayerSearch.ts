/** The rated players a TTA username search suggests, asked for only once the search is wanted. */
import { currentRun, leaderboard } from '@ttac/shared/ratings';
import { DEFAULT_VIEW, modelOf } from '../lib/leaderboard-view';
import type { SearchPlayer } from './SearchBox';
import { useLoad } from './useLoad';

/** usePlayerSearch returns the current run's rated players once wanted (the box was used, or already holds a name); none before, or while they load. */
export default function usePlayerSearch(apiUrl: string, wanted: boolean): SearchPlayer[] {
  const board = useLoad(apiUrl, wanted ? () => currentRun(apiUrl).then((run) => leaderboard(apiUrl, run.run_id, modelOf(DEFAULT_VIEW), DEFAULT_VIEW.format)) : null);
  return board.kind === 'ready' ? board.data.players : [];
}
