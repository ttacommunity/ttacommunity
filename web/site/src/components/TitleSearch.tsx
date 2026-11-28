/** TitleSearch is a search box answered by the API: it searches when submitted, never per keystroke, and emptying it clears the search. */
import { useEffect, useState } from 'react';
import './TitleSearch.css';

interface Props {
  /** What it searches, for screen readers, such as "Search video titles". */
  label: string;
  /** The search in force. */
  value: string;
  onSearch: (text: string) => void;
}

export default function TitleSearch({ label, value, onSearch }: Props) {
  const [typed, setTyped] = useState(value);
  useEffect(() => setTyped(value), [value]);
  return (
    <form
      role="search"
      className="title-search"
      onSubmit={(e) => {
        e.preventDefault();
        onSearch(typed);
      }}
    >
      <input
        type="search"
        className="input-antique"
        aria-label={label}
        maxLength={100}
        enterKeyHint="search"
        value={typed}
        onChange={(e) => {
          setTyped(e.target.value);
          if (e.target.value === '' && value !== '') onSearch('');
        }}
      />
      <button type="submit" className="btn-antique">
        Search
      </button>
    </form>
  );
}
