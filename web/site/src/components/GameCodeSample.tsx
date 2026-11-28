/** A game code field for the design page: type in any case and it shows in capitals. */
import { useState } from 'react';
import GameCodeField from '@ttac/shared/GameCodeField';

/** GameCodeSample is an empty share code field to type in. */
export default function GameCodeSample() {
  const [code, setCode] = useState('');
  return <GameCodeField label="Share code" value={code} onChange={setCode} />;
}
