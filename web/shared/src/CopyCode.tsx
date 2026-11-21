/** Text a reader may want to paste elsewhere, such as a game's replay code, with a button that copies it; a dash when there is none. */
import { useCopy } from './copy';

/** CopyCode shows code and copies it on a click: the button ticks once copied, and where the browser refuses, a note asks to select it. what names it for the button's label. */
export default function CopyCode({ code, className, what = 'replay code' }: { code: string | null; className?: string; what?: string }) {
  const { copied, copy } = useCopy();
  if (code === null) return <span className="copy-code-none">—</span>;
  return (
    <div className={className === undefined ? 'copy-code' : `copy-code ${className}`}>
      <code>{code}</code>
      <button type="button" className="copy-code-btn" onClick={() => copy(code)} title={`Copy ${what}`} aria-label={`Copy ${what} ${code}`}>
        {copied === true ? '✓' : '📋'}
      </button>
      <span className="copy-code-note" aria-live="polite">
        {copied === null ? '' : copied ? <span className="visually-hidden">Copied</span> : 'Select to copy'}
      </span>
    </div>
  );
}
