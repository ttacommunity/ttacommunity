/** A modal sheet: a framed dialog with its heading and a large close, for anything to read, choose or fill in above the page; and its titled sections. */
import { useId, type ReactNode, type RefObject } from 'react';
import { useModal } from './modal';

interface Props {
  /** The dialog, for the caller to close it from its own buttons. */
  dialog: RefObject<HTMLDialogElement | null>;
  /** Names the heading's id, `${id}-title`. */
  id: string;
  title: ReactNode;
  opener: HTMLElement;
  onClose: () => void;
  /** What the sheet holds, when that needs the wide sheet: a table or a video. */
  holds?: 'table' | 'video';
  /** The sheet's buttons, Cancel then its action, in a footer that stays below the body; a submit button names its form. */
  actions?: ReactNode;
  /** Buttons that reset or clear the sheet's form, at the footer's start. */
  resets?: ReactNode;
  /** The sheet's parts below its heading; a `sheet-body` scrolls. */
  children: ReactNode;
}

/** Sheet opens as it mounts and closes on Esc, the × or a click outside it; onClose runs as it closes. */
export default function Sheet({ dialog, id, title, opener, onClose, holds, actions, resets, children }: Props) {
  const closed = useModal(dialog, opener, onClose, { backdropCloses: true });
  return (
    <dialog ref={dialog} className={holds === undefined ? 'sheet' : 'sheet sheet-wide'} aria-labelledby={`${id}-title`} onClose={closed}>
      <div className="sheet-head">
        <h2 id={`${id}-title`}>{title}</h2>
        <button type="button" className="btn-close" aria-label="Close" onClick={() => dialog.current?.close()}>
          ×
        </button>
      </div>
      {children}
      {(actions !== undefined || resets !== undefined) && (
        <div className="sheet-foot">
          {resets !== undefined && <div className="sheet-foot-resets">{resets}</div>}
          {actions !== undefined && <div className="sheet-foot-actions">{actions}</div>}
        </div>
      )}
    </dialog>
  );
}

/** SheetSection is one titled part of a sheet's body, with a line above it; aside is a short note at the title's right. */
export function SheetSection({ title, aside, children }: { title: ReactNode; aside?: ReactNode; children: ReactNode }) {
  const id = useId();
  return (
    <section className="sheet-section" aria-labelledby={id}>
      <h3 id={id} className="sheet-section-title">
        <span>{title}</span>
        {aside !== undefined && <span className="sheet-section-aside">{aside}</span>}
      </h3>
      {children}
    </section>
  );
}
