/** useSave holds an admin box's save: its word and refusal, a run that sends, and a close for the refusal. */
import { useState } from 'react';
import type { AdminResult } from '../lib/admin-api';
import { refused } from '../lib/outcome';
import { IDLE, runSave, type SaveState } from '../lib/save';

/** Save is what useSave returns, and what SaveBar and SaveStatus show. */
export type Save = SaveState & {
  run: <T>(send: (() => Promise<AdminResult<T>>) | null, words: { done: string | ((data: T) => string); working?: string; at?: string }) => Promise<AdminResult<T> | null>;
  close: () => void;
  reset: () => void;
  /** refuse shows text as a refusal at at, for a change the box refuses itself. */
  refuse: (text: string, at: string) => void;
};

/** useSave starts idle; run is runSave on this box's state. */
export function useSave(): Save {
  const [state, setState] = useState<SaveState>(IDLE);
  return {
    ...state,
    run: (send, words) => runSave(send, words, setState),
    close: () => setState((s) => ({ ...s, refusal: null })),
    reset: () => setState(IDLE),
    refuse: (text, at) => setState({ status: '', refusal: refused(text, at), at }),
  };
}
