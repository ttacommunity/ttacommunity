/** SignInNote asks a visitor to sign in before taking part: an optional lead, then "Sign in to <to>", linking to the account page with the way back to this one. */
import { accountHref } from '../lib/account';
import Note from '@ttac/shared/Note';

export default function SignInNote({ lead, to }: { lead?: string; to: string }) {
  return (
    <Note>
      {lead && `${lead} `}
      <a href={accountHref(window.location.pathname, window.location.search)}>Sign in</a> to {to}
    </Note>
  );
}
