/** One way to say a page's data is loading, or could not load: a card for a page, a line in a panel, a row in a table. */
import { waitingText, type LoadState } from '../lib/waiting';
import Message from './Message';

interface Props {
  load: LoadState;
  /** What is loading, as it reads after "Loading", such as "this player". */
  what: string;
  form?: 'page' | 'inline' | 'row';
  /** The table's column count, for a row. */
  colSpan?: number;
}

/** Waiting shows nothing once loaded. */
export default function Waiting({ load, what, form = 'page', colSpan }: Props) {
  const text = waitingText(load, what);
  if (text === null) return null;
  return (
    <Message form={form} colSpan={colSpan} role={load.kind === 'error' ? 'alert' : 'status'}>
      {text}
    </Message>
  );
}
