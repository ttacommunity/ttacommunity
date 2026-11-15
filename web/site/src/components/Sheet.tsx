/** A modal sheet: a framed dialog with its heading and a large close button, for content too long for a short question. */
import type { ReactNode, RefObject } from 'react';
import { useModal } from '@ttac/shared/modal';

interface Props {
  /** The dialog, for the caller to close it from its own buttons. */
  dialog: RefObject<HTMLDialogElement | null>;
  /** Names the heading's id, `${id}-title`. */
  id: string;
  title: ReactNode;
  opener: HTMLElement;
  onClose: () => void;
  /** Added to `sheet`, for the sheet's own width or parts. */
  className?: string;
  /** The sheet's parts below its heading; a `sheet-body` scrolls. */
  children: ReactNode;
}

/** Sheet opens as it mounts and closes on Esc, the × or a click outside it; onClose runs as it closes. */
export default function Sheet({ dialog, id, title, opener, onClose, className, children }: Props) {
  const closed = useModal(dialog, opener, onClose, { backdropCloses: true });
  return (
    <dialog ref={dialog} className={className === undefined ? 'sheet' : `sheet ${className}`} aria-labelledby={`${id}-title`} onClose={closed}>
      <div className="sheet-head">
        <h2 id={`${id}-title`}>{title}</h2>
        <button type="button" className="sheet-close" aria-label="Close" onClick={() => dialog.current?.close()}>
          ×
        </button>
      </div>
      {children}
    </dialog>
  );
}
