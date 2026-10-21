/** The page an emailed sign-in link opens: the player presses to finish, so a mail scanner opening the link spends nothing, then goes back where they were. */
import { useState } from 'react';
import { browserStorage, rememberSignedIn } from '../lib/account';
import { emailFinishTarget, finishEmailLink, tokenFromHash } from '../lib/email-sign-in';
import './Account.css';

type Shown = { kind: 'ready' } | { kind: 'busy' } | { kind: 'expired' | 'refused'; text: string };

/** EmailLinkPage finishes signing in with the token in the page's fragment. */
export default function EmailLinkPage({ apiUrl }: { apiUrl: string }) {
  const [token] = useState(() => tokenFromHash(window.location.hash));
  const [shown, setShown] = useState<Shown>({ kind: 'ready' });
  const finish = async () => {
    if (token === null) return;
    setShown({ kind: 'busy' });
    const got = await finishEmailLink(apiUrl, token);
    if (got.kind !== 'done') {
      setShown({ kind: got.kind, text: got.message });
      return;
    }
    rememberSignedIn(browserStorage(), true);
    window.location.replace(emailFinishTarget(got.outcome, got.back));
  };
  return (
    <section className="rating-card account-card" aria-labelledby="email-finish">
      <h2 id="email-finish" className="rating-card-heading">
        Finish signing in
      </h2>
      {token === null ? (
        <p>
          This page opens from the link in a sign-in email. <a href="/account/">Ask for a link on your account page</a>.
        </p>
      ) : (
        <>
          <p>You opened the link from your sign-in email. It works once.</p>
          <div className="account-row">
            <button type="button" className="btn-antique" disabled={shown.kind === 'busy'} onClick={() => void finish()}>
              {shown.kind === 'busy' ? 'Signing in…' : 'Sign in'}
            </button>
          </div>
        </>
      )}
      {(shown.kind === 'expired' || shown.kind === 'refused') && (
        <p className="account-problem" role="alert">
          {shown.kind === 'expired' ? 'This link has expired or has been used. ' : `${shown.text} `}
          <a href="/account/">Ask for a new link</a>.
        </p>
      )}
    </section>
  );
}
