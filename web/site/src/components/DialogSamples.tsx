/** The design page's dialog samples: a question to confirm, one that cannot be undone, one wanting a typed answer, and sheets: plain, with sections and buttons, and wide. */
import { useRef, useState } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import Sheet, { SheetSection } from '@ttac/shared/Sheet';

type Kind = 'plain' | 'form' | 'wide';

/** DialogSamples opens each kind of dialog and says what it answered. */
export default function DialogSamples() {
  const dialog = useDialog();
  const [sheet, setSheet] = useState<{ kind: Kind; opener: HTMLElement } | null>(null);
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
  const open = (kind: Kind) => (e: React.MouseEvent<HTMLButtonElement>) => setSheet({ kind, opener: e.currentTarget });
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
      <button type="button" className="btn-antique" onClick={open('plain')}>
        A sheet
      </button>
      <button type="button" className="btn-antique" onClick={open('form')}>
        A sheet with buttons
      </button>
      <button type="button" className="btn-antique" onClick={open('wide')}>
        A wide sheet
      </button>
      {sheet !== null && <SampleSheet kind={sheet.kind} opener={sheet.opener} onClose={() => setSheet(null)} onAnswer={setAnswer} />}
      <span role="status">{answer}</span>
    </div>
  );
}

/** SampleSheet is one sample sheet of the kind asked for. */
function SampleSheet({ kind, opener, onClose, onAnswer }: { kind: Kind; opener: HTMLElement; onClose: () => void; onAnswer: (a: string) => void }) {
  const box = useRef<HTMLDialogElement>(null);
  const done = (a: string) => {
    onAnswer(a);
    box.current?.close();
  };
  if (kind === 'form') {
    return (
      <Sheet
        dialog={box}
        id="sample-form"
        title="Sample options"
        opener={opener}
        onClose={onClose}
        resets={
          <button type="button" className="btn-quiet" onClick={() => onAnswer('Reset.')}>
            Reset
          </button>
        }
        actions={
          <>
            <button type="button" className="btn-quiet" onClick={() => box.current?.close()}>
              Cancel
            </button>
            <button type="button" className="btn-antique" onClick={() => done('Applied.')}>
              Apply
            </button>
          </>
        }
      >
        <div className="sheet-body">
          <p className="sheet-intro">A line saying what the sheet is for. Its buttons stay below as the body scrolls.</p>
          <SheetSection title="A section" aside="A short note">
            <p>A section of the sheet, with a line above it.</p>
          </SheetSection>
          <SheetSection title="Another section">
            <p>Another.</p>
          </SheetSection>
        </div>
      </Sheet>
    );
  }
  if (kind === 'wide') {
    return (
      <Sheet dialog={box} id="sample-wide" title="Sample wide sheet" opener={opener} onClose={onClose} holds="table">
        <div className="sheet-body">
          <p>A sheet that holds a table or a video is wider.</p>
        </div>
      </Sheet>
    );
  }
  return (
    <Sheet dialog={box} id="sample-sheet" title="Sample sheet" opener={opener} onClose={onClose}>
      <div className="sheet-body">
        <p>A sheet holds more than a question: a list, a table or a form. Esc, the × or a click outside closes it.</p>
      </div>
    </Sheet>
  );
}
