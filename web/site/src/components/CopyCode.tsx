/** A game's replay code with a button that copies it, or a dash for a game without one. */
import { useCopy } from '@ttac/shared/copy';
import './CopyCode.css';

/** CopyCode shows code and copies it on a click: the button ticks once copied, and where the browser refuses, a note asks to select it. */
export default function CopyCode({ code, className }: { code: string | null; className?: string }) {
  const { copied, copy } = useCopy();
  if (code === null) return <span className="replay-none">—</span>;
  return (
    <div className={className === undefined ? 'replay' : `replay ${className}`}>
      <code>{code}</code>
      <button type="button" className="btn-copy-code" onClick={() => copy(code)} title="Copy replay code" aria-label={`Copy replay code ${code}`}>
        {copied === true ? '✓' : '📋'}
      </button>
      <span className="replay-note" aria-live="polite">
        {copied === null ? '' : copied ? <span className="visually-hidden">Copied</span> : 'Select to copy'}
      </span>
    </div>
  );
}
