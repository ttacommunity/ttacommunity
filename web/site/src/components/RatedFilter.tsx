/** Two mixable toggle buttons for rated and unrated games, announced to screen readers when changed. */
import { useState } from 'react';
import { ratedFilterStatus, type GameRatingFilter } from '../lib/career';

/** RatedFilter lets the viewer independently toggle rated and unrated games. */
export default function RatedFilter({
  value,
  onChange,
  className = '',
}: {
  value: GameRatingFilter;
  onChange: (next: GameRatingFilter) => void;
  className?: string;
}) {
  const [said, setSaid] = useState('');
  const toggle = (key: keyof GameRatingFilter) => {
    const next = { ...value, [key]: !value[key] };
    onChange(next);
    setSaid(ratedFilterStatus(next));
  };

  return (
    <>
      <div className={`rated-filter-group ${className}`} role="group" aria-label="Game rating filter">
        <button
          type="button"
          className={`btn-antique unrated-toggle-btn rated-filter-btn ${value.rated ? 'active' : ''}`}
          aria-pressed={value.rated}
          onClick={() => toggle('rated')}
          title={value.rated ? 'Rated tournament games shown (click to hide)' : 'Rated tournament games hidden (click to show)'}
        >
          <span aria-hidden="true">🏆</span> Rated
        </button>
        <button
          type="button"
          className={`btn-antique unrated-toggle-btn rated-filter-btn ${value.unrated ? 'active' : ''}`}
          aria-pressed={value.unrated}
          onClick={() => toggle('unrated')}
          title={value.unrated ? 'Unrated games shown (click to hide)' : 'Unrated games hidden (click to show)'}
        >
          <span aria-hidden="true">🌐</span> Unrated
        </button>
      </div>
      <span className="visually-hidden" role="status">
        {said}
      </span>
    </>
  );
}
