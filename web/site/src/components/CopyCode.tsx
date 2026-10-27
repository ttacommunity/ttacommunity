/** A game's replay code with a button that copies it, or a dash for a game without one; styled by PlayerPage.css. */
import { useCopy } from '@ttac/shared/copy';

/** CopyCode shows code and copies it on a click, ticking the button while it is copied. */
export default function CopyCode({ code }: { code: string | null }) {
  const { copied, copy } = useCopy();
  if (code === null) return <span className="replay-none">—</span>;
  return (
    <div className="replay">
      <code>{code}</code>
      <button type="button" className="btn-copy-code" onClick={() => copy(code)} title="Copy replay code" aria-label="Copy replay code">
        {copied === true ? '✓' : '📋'}
      </button>
    </div>
  );
}
