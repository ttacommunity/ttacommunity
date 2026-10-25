/** Opening a `<dialog>` as a modal, and giving the focus back to what opened it as it closes. */
import { useEffect, type RefObject, type SyntheticEvent } from 'react';

/** Opener is the control that opened a dialog: the button clicked, as its handler's `e.currentTarget`. */
export interface Opener {
  isConnected: boolean;
  focus(): void;
}

/** focusBack focuses opener as its dialog closes, unless there is none or it has left the page. */
export function focusBack(opener: Opener | null) {
  if (opener?.isConnected === true) opener.focus();
}

/**
 * useModal opens box as a modal as it mounts, and returns its close handler: onClose, after the focus goes back to
 * opener, which every browser needs told (Safari and Firefox on macOS do not focus a button on click). Pass it as the
 * dialog's onClose. With backdropCloses, a click outside closes it.
 */
export function useModal(
  box: RefObject<HTMLDialogElement | null>,
  opener: Opener | null,
  onClose: (e: SyntheticEvent<HTMLDialogElement>) => void,
  { backdropCloses = false } = {},
) {
  useEffect(() => {
    const d = box.current;
    if (d === null) return;
    d.showModal();
    const onClick = (e: MouseEvent) => backdropCloses && e.target === d && d.close();
    d.addEventListener('click', onClick);
    return () => d.removeEventListener('click', onClick);
  }, []);
  return (e: SyntheticEvent<HTMLDialogElement>) => {
    focusBack(opener);
    onClose(e);
  };
}
