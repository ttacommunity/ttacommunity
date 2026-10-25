/** The design page's dialog samples: a question to confirm, one that cannot be undone, and one wanting a typed answer. */
import { useState } from 'react';
import { useDialog } from '@ttac/shared/Dialog';

/** DialogSamples opens each kind of dialog and says what it answered. */
export default function DialogSamples() {
  const dialog = useDialog();
  const [answer, setAnswer] = useState('');
  const confirm = async (opener: HTMLElement) =>
    setAnswer((await dialog.confirm({ heading: 'Publish run 12?', text: 'The site shows it at once.', action: 'Publish' }, opener)) ? 'Published.' : 'Cancelled.');
  const final = async (opener: HTMLElement) => {
    const sure = await dialog.confirm({ heading: 'Delete run 12?', text: 'Every result of it is deleted too. This cannot be undone.', action: 'Delete run', final: true }, opener);
    setAnswer(sure ? 'Deleted.' : 'Cancelled.');
  };
  const ask = async (opener: HTMLElement) => {
    const reason = await dialog.ask({ heading: 'Ban Sample Player One?', text: 'A banned member cannot be entered in an edition.', field: 'The reason', required: true, action: 'Ban' }, opener);
    setAnswer(reason === null ? 'Cancelled.' : `Banned: ${reason}`);
  };
  return (
    <div className="dialog-samples">
      {dialog.element}
      <button type="button" className="btn-antique" onClick={(e) => void confirm(e.currentTarget)}>
        Confirm
      </button>
      <button type="button" className="btn-antique" onClick={(e) => void final(e.currentTarget)}>
        Cannot be undone
      </button>
      <button type="button" className="btn-antique" onClick={(e) => void ask(e.currentTarget)}>
        With a reason
      </button>
      <span role="status">{answer}</span>
    </div>
  );
}
