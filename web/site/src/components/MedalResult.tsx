/** A tournament result: a medal's in a badge of its colour with the tournament's kind inside, any other placement as plain text. */
import type { TournamentRecord } from '@ttac/shared/ratings';
import { formatTournamentDisplay, resultCell } from '../lib/achievements';
import './MedalResult.css';

/** MedalResult shows record r's result, a medal badge when it won one. */
export default function MedalResult({ r }: { r: TournamentRecord }) {
  if (r.medal === null) return <span className="medal-result-plain">{r.placement || '-'}</span>;
  const medal = r.medal.charAt(0).toUpperCase() + r.medal.slice(1);
  return (
    <span className={`medal-result medal-result-${r.medal}`} title={`${formatTournamentDisplay(r.tournament)} ${medal} (${r.placement})`}>
      {resultCell(r)}
    </span>
  );
}
