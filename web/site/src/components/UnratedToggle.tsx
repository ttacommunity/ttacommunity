/** The player page's button that adds the run's unrated games to a panel, announced to screen readers when pressed. */
import { useState } from 'react';

/** UnratedToggle shows whether unrated games are included and flips it; `className` places it in its panel. */
export default function UnratedToggle({
  on,
  onChange,
  className = '',
}: {
  on: boolean;
  onChange: (next: boolean) => void;
  className?: string;
}) {
  const [said, setSaid] = useState('');
  const flip = () => {
    onChange(!on);
    setSaid(on ? 'Showing rated games only' : 'Showing all games, unrated included');
  };
  return (
    <>
      <button
        type="button"
        className={`btn-antique unrated-toggle-btn ${on ? 'active' : ''} ${className}`}
        onClick={flip}
        title={on ? 'Showing all finished games, rated or not' : 'Showing rated games only'}
      >
        {/* The label states what is shown, so it is not aria-pressed (WAI-ARIA APG, button pattern) */}
        <span aria-hidden="true">{on ? '🌐' : '🏆'}</span> {on ? 'Filter: All Games' : 'Filter: Rated Games Only'}
      </button>
      <span className="visually-hidden" role="status">
        {said}
      </span>
    </>
  );
}
