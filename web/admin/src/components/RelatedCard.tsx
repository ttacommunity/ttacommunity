/** Where a tournament or group sits: the groups it is in, as links, and for a tournament the Move button. */
import type { AdminTournament } from '../lib/admin-api';
import { groupsAbove } from '../lib/tournament-groups';
import { settingsHref } from '../lib/tournament-page';
import MoveButton from './MoveButton';

/** RelatedCard shows the groups t is in, from all; a tournament (a group moves from its settings) can be moved. */
export default function RelatedCard({
  t,
  all,
  onReload,
}: {
  t: AdminTournament;
  all: readonly AdminTournament[];
  onReload: () => Promise<void>;
}) {
  const above = groupsAbove(all, t.slug);
  return (
    <p className="group-place">
      <a href="/">Tournaments</a>
      {above.map((g) => (
        <span key={g.slug}>
          {' › '}
          <a href={settingsHref(g.slug)}>{g.name}</a>
        </span>
      ))}
      {' › '}
      <strong>{t.name}</strong>
      {!t.is_group && (
        <>
          {' '}
          <MoveButton t={t} all={all} onReload={onReload} />
        </>
      )}
    </p>
  );
}
