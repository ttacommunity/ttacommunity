/** A feed's options in an endpoint's sheet: for Twitch, categories and keywords to match, as comma-separated lists, whether a stream needs both or either, and "was live"; for RSS, keywords only. */
import { useState } from 'react';
import type { components } from '@ttac/shared/api';
import { words } from '../lib/feeds';

type FeedOption = components['schemas']['FeedOption'];
type Match = NonNullable<FeedOption['match']>;

interface Props {
  /** twitch or rss: the kind of feed, which sets the options offered. */
  kind: 'twitch' | 'rss';
  options: FeedOption;
  /** Runs with the options as changed, the lists read from what was typed. */
  onChange: (change: Partial<Omit<FeedOption, 'feed'>>) => void;
}

export default function FeedOptions({ kind, options, onChange }: Props) {
  const [categories, setCategories] = useState(options.categories.join(', '));
  const [keywords, setKeywords] = useState(options.keywords.join(', '));
  if (kind === 'rss') {
    return (
      <div className="feed-options">
        <label>
          With words in the title or summary
          <input className="input-antique" value={keywords} placeholder="Any; such as TTA, tournament"
            onChange={(e) => {
              setKeywords(e.target.value);
              onChange({ keywords: words(e.target.value) });
            }} />
        </label>
      </div>
    );
  }
  return (
    <div className="feed-options">
      <label>
        Only in categories
        <input className="input-antique" value={categories} placeholder="Any; such as Through the Ages"
          onChange={(e) => {
            setCategories(e.target.value);
            onChange({ categories: words(e.target.value) });
          }} />
      </label>
      <label>
        With words in the title
        <input className="input-antique" value={keywords} placeholder="Any; such as TTA, tournament"
          onChange={(e) => {
            setKeywords(e.target.value);
            onChange({ keywords: words(e.target.value) });
          }} />
      </label>
      <label>
        Post a stream that passes{' '}
        <select className="input-antique" value={options.match ?? 'all'} onChange={(e) => onChange({ match: e.target.value as Match })}>
          <option value="all">both lists</option>
          <option value="any">either list</option>
        </select>
      </label>
      <label>
        <input type="checkbox" checked={options.was_live} onChange={(e) => onChange({ was_live: e.target.checked })} /> Edit to "was live" when the stream ends
      </label>
    </div>
  );
}
