/** Copying a replay code: to the clipboard, with a short note of how it went that goes when the button does. */
import { useEffect, useRef, useState } from 'react';

/** copyText writes text to clipboard and says whether it could; a page not served over HTTPS has no clipboard. */
export async function copyText(clipboard: Pick<Clipboard, 'writeText'> | undefined, text: string): Promise<boolean> {
  if (clipboard === undefined) return false;
  try {
    await clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

/** useCopy returns copy, and copied: whether the last copy worked, for 1.5 s after it, else null. */
export function useCopy() {
  const [copied, setCopied] = useState<boolean | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const live = useRef(true);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
      clearTimeout(timer.current);
    };
  }, []);
  const copy = (text: string) => {
    void copyText(navigator.clipboard as Clipboard | undefined, text).then((ok) => {
      if (!live.current) return;
      setCopied(ok);
      clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(null), 1500);
    });
  };
  return { copied, copy };
}
