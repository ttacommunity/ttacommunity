/** Upstream's player search: a text field with suggestions as you type, for one name or a comma list. */
import { useEffect, useId, useState } from 'react';
import type { LeaderboardRow } from '@ttac/shared/ratings';
import { suggest } from '../lib/leaderboard-view';

interface Props {
  players: readonly LeaderboardRow[];
  value: string;
  onSearch: (term: string) => void;
}

/** flagEmoji turns a country code into its flag emoji, as upstream's suggestions show it. */
function flagEmoji(code: string | null): string {
  if (code === null || !/^[A-Z]{2}$/.test(code)) return '';
  return String.fromCodePoint(...[...code].map((c) => 127397 + c.charCodeAt(0)));
}

/** Highlighted shows a name with the typed part underlined, as text. */
function Highlighted({ name, query }: { name: string; query: string }) {
  const at = name.toLowerCase().indexOf(query.toLowerCase());
  if (query === '' || at < 0) return <>{name}</>;
  return (
    <>
      {name.slice(0, at)}
      <span className="search-match">{name.slice(at, at + query.length)}</span>
      {name.slice(at + query.length)}
    </>
  );
}

/** SearchBox searches on Enter or 🔍, and fills in a suggestion when one is picked. */
export default function SearchBox({ players, value, onSearch }: Props) {
  const [text, setText] = useState(value);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const listId = useId();
  const query = (text.split(',').pop() ?? '').trim();
  const found = open ? suggest(players, text) : [];

  useEffect(() => setText(value), [value]);

  const pick = (name: string) => {
    const parts = text.split(',');
    parts[parts.length - 1] = parts.length > 1 ? ` ${name}` : name;
    const term = parts.join(',').trim();
    setText(term);
    setOpen(false);
    onSearch(term);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (found.length === 0) return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const step = e.key === 'ArrowDown' ? 1 : -1;
      setActive((i) => (i + step + found.length) % found.length);
    } else if ((e.key === 'Enter' || e.key === 'Tab') && active >= 0) {
      e.preventDefault();
      pick(found[active].name);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setOpen(false);
    }
  };

  return (
    <form
      className="search-form"
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        setOpen(false);
        onSearch(text.trim());
      }}
    >
      <div className="search-input-wrapper">
        <input
          type="text"
          className="input-antique"
          placeholder="Search player ..."
          title="Separate multiple names with commas."
          aria-label="Search player"
          autoComplete="off"
          role="combobox"
          aria-expanded={found.length > 0}
          aria-controls={listId}
          aria-activedescendant={active >= 0 && found.length > 0 ? `${listId}-${active}` : undefined}
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            setOpen(true);
            setActive(-1);
          }}
          onFocus={() => setOpen(true)}
          onBlur={() => setOpen(false)}
          onKeyDown={onKeyDown}
        />
        <button type="submit" className="search-submit" title="Search" aria-label="Search">
          🔍
        </button>
        {found.length > 0 && (
          <div className="search-suggestions">
            <div id={listId} role="listbox" aria-label="Players">
            {found.map((p, i) => (
              <div
                key={p.name}
                id={`${listId}-${i}`}
                role="option"
                tabIndex={-1}
                aria-selected={i === active}
                className={i === active ? 'search-suggestion active' : 'search-suggestion'}
                onMouseEnter={() => setActive(i)}
                onMouseDown={(e) => {
                  e.preventDefault();
                  pick(p.name);
                }}
              >
                <span className="search-suggestion-left">
                  {flagEmoji(p.country) && <span className="flag-icon">{flagEmoji(p.country)}</span>}
                  <span className="search-suggestion-name">
                    <Highlighted name={p.name} query={query} />
                  </span>
                </span>
                {p.title && <span className={`badge badge-${p.title.code.toLowerCase()}`}>{p.title.code}</span>}
              </div>
            ))}
            </div>
            <div className="search-suggestions-hint" aria-hidden="true">
              <span>&uarr;&darr; navigate</span>
              <span>&crarr; select</span>
              <span>ESC close</span>
            </div>
          </div>
        )}
      </div>
      {value !== '' && (
        <button type="button" className="btn-reset search-clear" title="Clear search" onClick={() => onSearch('')}>
          ✕
        </button>
      )}
    </form>
  );
}
