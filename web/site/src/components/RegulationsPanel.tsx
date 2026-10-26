/** A tournament's Format & Regulations, as DANeo's series pages show it: bullets, and how its games are rated. */
import type { RegulationsBox } from '../lib/catalogue-view';

/** RegulationsPanel shows the bullets, where they come from when that is CGE, and the rating note for a rated tournament. */
export default function RegulationsPanel({ box, rated }: { box: RegulationsBox; rated: boolean }) {
  return (
    <div className="panel-card">
      <h2 className="panel-header">
        <span>📜</span> Format &amp; Regulations
      </h2>
      <ul className="rules-list">
        {box.lines.map((line, i) => (
          <li key={`${i} ${line}`}>{line}</li>
        ))}
      </ul>
      {box.source !== null && (
        <p className="rules-source">
          From{' '}
          <a href={box.source.url} target="_blank" rel="noopener noreferrer" className="cge-link">
            {box.source.name} on CGE ↗
          </a>
        </p>
      )}
      {rated && (
        <div className="rules-note">
          <strong>Rating Integration:</strong> Matches played in this circuit are parsed with match integrity validation and
          automatically update player Glicko-2 parameters according to their multiplayer weighting rules.
        </div>
      )}
    </div>
  );
}
