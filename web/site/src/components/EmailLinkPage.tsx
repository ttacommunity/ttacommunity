/** The page an emailed sign-in link opens: the player presses to finish, so a mail scanner opening the link spends nothing, then goes back where they were. */
import { useState } from 'react';
import { rememberSignedIn } from '../lib/account';
import { browserStorage } from '@ttac/shared/storage';
import { emailFinishTarget, finishEmailLink, tokenFromHash } from '../lib/email-sign-in';
import './Account.css';
import OutcomeNotice from '@ttac/shared/OutcomeNotice';
import Panel from './Panel';

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
    <Panel id="email-finish" title="Finish signing in" className="account-card">
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
        <OutcomeNotice
          kind="refused"
          text={
            <>
              {shown.kind === 'expired' ? 'This link has expired or has been used. ' : `${shown.text} `}
              <a href="/account/">Ask for a new link</a>.
            </>
          }
          onClose={() => setShown({ kind: 'ready' })}
        />
      )}
    </Panel>
  );
}
