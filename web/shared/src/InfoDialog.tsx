/** A modal that shows something to read, with Close and any buttons of the caller's: the page behind is inert, Escape closes it, and focus goes back to what opened it. */
import { useEffect, useId, useRef, type ReactNode } from 'react';

/** InfoDialog opens title and children as a modal at once, and calls onClose when it closes; actions go before Close. */
export default function InfoDialog({ title, onClose, actions, children }: { title: string; onClose: () => void; actions?: ReactNode; children: ReactNode }) {
  const box = useRef<HTMLDialogElement>(null);
  const id = useId();
  useEffect(() => {
    box.current?.showModal();
  }, []);
  return (
    <dialog ref={box} className="site-dialog" aria-labelledby={`${id}-heading`} onClose={onClose}>
      <h2 id={`${id}-heading`} className="site-dialog-heading">
        {title}
      </h2>
      {children}
      <div className="site-dialog-buttons">
        {actions}
        <button type="button" className="btn-antique" onClick={() => box.current?.close()}>
          Close
        </button>
      </div>
    </dialog>
  );
}
