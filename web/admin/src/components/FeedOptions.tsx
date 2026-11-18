/** A Twitch feed's options in an endpoint's sheet: categories and keywords to match, as comma-separated lists, and "was live". */
import { useState } from 'react';
import type { components } from '@ttac/shared/api';
import { words } from '../lib/feeds';

type FeedOption = components['schemas']['FeedOption'];

interface Props {
  options: FeedOption;
  /** Runs with the options as changed, the lists read from what was typed. */
  onChange: (change: Partial<Omit<FeedOption, 'feed'>>) => void;
}

export default function FeedOptions({ options, onChange }: Props) {
  const [categories, setCategories] = useState(options.categories.join(', '));
  const [keywords, setKeywords] = useState(options.keywords.join(', '));
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
        Or with words in the title
        <input className="input-antique" value={keywords} placeholder="Any; such as TTA, tournament"
          onChange={(e) => {
            setKeywords(e.target.value);
            onChange({ keywords: words(e.target.value) });
          }} />
      </label>
      <label>
        <input type="checkbox" checked={options.was_live} onChange={(e) => onChange({ was_live: e.target.checked })} /> Edit to "was live" when the stream ends
      </label>
    </div>
  );
}
