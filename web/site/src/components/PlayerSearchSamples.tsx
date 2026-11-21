/** The player search on /design: the one box every page uses, with invented players to suggest. */
import { useState } from 'react';
import PlayerSearch, { type SearchPlayer } from './PlayerSearch';

const PLAYERS: SearchPlayer[] = [{ name: 'Sample Player' }, { name: 'Sample Rival' }, { name: 'Another Sample' }];

/** PlayerSearchSamples shows the box as a page's filter would use it: type to see suggestions. */
export default function PlayerSearchSamples() {
  const [text, setText] = useState('');
  return <PlayerSearch players={PLAYERS} value={text} onChange={setText} />;
}
