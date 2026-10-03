/** One tournament (or circuit) card, as the Hall of Fame and Every Tournament both show them. */

/** TournamentCard links to a tournament's page, with its icon, badge and summary where known, and its figures. */
export default function TournamentCard({
  href,
  name,
  icon,
  badge,
  summary,
  rows,
  cta,
}: {
  href: string;
  name: string;
  icon?: string;
  badge?: string;
  summary?: string;
  rows: [string, string][];
  cta: string;
}) {
  return (
    <a href={href} className="series-card">
      <div>
        {(icon !== undefined || badge !== undefined) && (
          <div className="series-card-top">
            <div className="series-icon">{icon ?? '📜'}</div>
            {badge !== undefined && <div className="series-badge">{badge}</div>}
          </div>
        )}
        <div className="series-name">{name}</div>
        {summary !== undefined && <div className="series-summary">{summary}</div>}
        {rows.map(([label, value]) => (
          <div key={label} className="series-meta-row">
            <span>{label}</span>
            <span>{value}</span>
          </div>
        ))}
      </div>
      <div className="series-cta">
        <span>{cta}</span>
        <span className="arrow">&rarr;</span>
      </div>
    </a>
  );
}
