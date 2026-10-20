/** Cloudflare's Turnstile widget for the email form: its script loaded once from Cloudflare, each token passed up, and an empty one when a token expires or the check fails. */
import { useEffect, useRef, useState } from 'react';
import { TURNSTILE_ACTION, TURNSTILE_SCRIPT } from '../lib/email-sign-in';

type Widget = {
  render(container: HTMLElement, options: Record<string, unknown>): string;
  remove(id: string): void;
};

declare global {
  interface Window {
    turnstile?: Widget;
  }
}

let loading: Promise<Widget> | null = null;

/** loadTurnstile adds Turnstile's script to the page once, and resolves to its widget. */
function loadTurnstile(): Promise<Widget> {
  loading ??= new Promise<Widget>((resolve, reject) => {
    const script = document.createElement('script');
    script.src = TURNSTILE_SCRIPT;
    script.onload = () => (window.turnstile === undefined ? reject(new Error('no widget')) : resolve(window.turnstile));
    script.onerror = () => reject(new Error('Turnstile did not load'));
    document.head.appendChild(script);
  }).catch((e: unknown) => {
    loading = null;
    throw e;
  });
  return loading;
}

/** Turnstile renders the widget for sitekey; a new round renders it afresh, as each token is used once. */
export default function Turnstile({ sitekey, round, onToken }: { sitekey: string; round: number; onToken: (token: string) => void }) {
  const box = useRef<HTMLDivElement>(null);
  const [failed, setFailed] = useState(false);
  // The widget calls back whichever handler is current, so a new one does not render it again.
  const handler = useRef(onToken);
  useEffect(() => {
    handler.current = onToken;
  });
  useEffect(() => {
    const onToken = (token: string) => handler.current(token);
    let widget: Widget | undefined;
    let id: string | undefined;
    let gone = false;
    onToken('');
    loadTurnstile()
      .then((w) => {
        if (gone || box.current === null) return;
        widget = w;
        id = w.render(box.current, {
          sitekey,
          action: TURNSTILE_ACTION,
          theme: 'dark',
          callback: (token: string) => onToken(token),
          'expired-callback': () => onToken(''),
          'error-callback': () => onToken(''),
        });
      })
      .catch(() => setFailed(true));
    return () => {
      gone = true;
      if (widget !== undefined && id !== undefined) widget.remove(id);
    };
  }, [sitekey, round]);
  return (
    <>
      <div ref={box} />
      {failed && <p className="account-problem">The check that you are a person did not load. Reload the page to try again.</p>}
    </>
  );
}
