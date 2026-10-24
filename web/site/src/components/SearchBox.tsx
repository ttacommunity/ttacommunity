/** Upstream's player search: a text field with suggestions as you type, for one name or a comma list. */
import { useEffect, useId, useRef, useState } from 'react';
import type { LeaderboardRow } from '@ttac/shared/ratings';
import { suggest } from '../lib/leaderboard-view';
import './SearchBox.css';

/** A player the box can suggest: their name, and the flag and title shown beside it where known. */
export type SearchPlayer = { name: string; country?: string | null; title?: LeaderboardRow['title'] };

interface Props {
  players: readonly SearchPlayer[];
  value: string;
  /** Called with the term searched; in field mode the page's own form searches instead. */
  onSearch?: (term: string) => void;
  /** Empties the box after each search, for adding names one at a time. */
  clearOnSearch?: boolean;
  /** Field mode: the box is one field of the page's own form, sent under this name. */
  name?: string;
  inputClassName?: string;
  placeholder?: string;
  /** Fills the width it is given, for long names. */
  wide?: boolean;
  /** Fill mode: one name, a field of the page's own form; onChange gets each edit and the name picked, and nothing is searched. */
  onChange?: (text: string) => void;
  /** The input's id, for the page's own label. */
  id?: string;
}

/** flagEmoji turns a country code into its flag emoji, as upstream's suggestions show it. */
function flagEmoji(code: string | null | undefined): string {
  if (code == null || !/^[A-Z]{2}$/.test(code)) return '';
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

/** SearchBox searches on Enter or 🔍, and fills in a suggestion when one is picked; in field mode, the page's form submits. */
export default function SearchBox({
  players,
  value,
  onSearch,
  clearOnSearch = false,
  name,
  inputClassName = 'input-antique',
  placeholder = 'Search player ...',
  wide = false,
  onChange,
  id,
}: Props) {
  const fill = onChange !== undefined;
  const field = name !== undefined;
  const input = useRef<HTMLInputElement>(null);
  const [text, setText] = useState(value);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const listId = useId();
  const query = (text.split(',').pop() ?? '').trim();
  const found = open ? suggest(players, text) : [];

  useEffect(() => setText(value), [value]);

  const search = (term: string) => {
    onSearch?.(term);
    if (clearOnSearch) setText('');
  };

  const pick = (name: string) => {
    if (fill) {
      setText(name);
      setOpen(false);
      onChange(name);
      return;
    }
    const parts = text.split(',');
    parts[parts.length - 1] = parts.length > 1 ? ` ${name}` : name;
    const term = parts.join(',').trim();
    setText(term);
    setOpen(false);
    if (field && input.current !== null) {
      input.current.value = term;
      input.current.form?.requestSubmit();
    } else search(term);
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

  const box = (
    <div className={wide ? 'search-input-wrapper wide' : 'search-input-wrapper'}>
      <input
        type="text"
        ref={input}
        id={id}
        name={name}
        className={inputClassName}
        placeholder={placeholder}
        title={fill ? undefined : 'Separate multiple names with commas.'}
        aria-label={id === undefined ? 'Search player' : undefined}
        autoComplete="off"
        enterKeyHint="search"
        role="combobox"
        aria-expanded={found.length > 0}
        aria-controls={listId}
        aria-activedescendant={active >= 0 && found.length > 0 ? `${listId}-${active}` : undefined}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          onChange?.(e.target.value);
          setOpen(true);
          setActive(-1);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={onKeyDown}
      />
      {!field && !fill && (
        <button type="submit" className="search-submit" title="Search" aria-label="Search">
          🔍
        </button>
      )}
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
  );
  if (field || fill) return box;
  return (
    <form
      className="search-form"
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        setOpen(false);
        search(text.trim());
      }}
    >
      {box}
      {value !== '' && (
        <button type="button" className="search-clear" title="Clear search" onClick={() => onSearch?.('')}>
          ✕
        </button>
      )}
    </form>
  );
}
