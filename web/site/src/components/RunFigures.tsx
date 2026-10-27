/** The home page's live figures from the current run: its players, its matches and when it was updated. */
import { currentRun } from '@ttac/shared/ratings';
import { count } from '@ttac/shared/numbers';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import { useLoad } from './useLoad';

export default function RunFigures({ apiUrl }: { apiUrl: string }) {
  const load = useLoad(apiUrl, () => currentRun(apiUrl));
  if (load.kind !== 'ready') return null;
  const run = load.data;
  return (
    <div className="home-stats-bar">
      {run.players !== null && (
        <div className="home-stat-pill">
          Active Players: <strong>{count(run.players)}</strong>
        </div>
      )}
      {run.games !== null && (
        <div className="home-stat-pill">
          Recorded Matches: <strong>{count(run.games)}</strong>
        </div>
      )}
      {run.finished_at && (
        <div className="home-stat-pill">
          Updated: <strong>{formatInstant(run.finished_at, viewerZone())}</strong>
        </div>
      )}
    </div>
  );
}
