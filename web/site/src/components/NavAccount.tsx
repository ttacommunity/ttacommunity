/** The nav's account link: Sign in, or who is signed in, leading to the account page with the way back to this one. It asks the api only in a browser that has signed in. */
import { useEffect, useState } from 'react';
import { accountHref, browserStorage, navLabel, rememberSignedIn, seemsSignedIn, sharedMe, withoutSignInParam } from '../lib/account';

/** NavAccount marks a sign-in that just finished, then names whoever is signed in. */
export default function NavAccount({ apiUrl, current }: { apiUrl: string; current: boolean }) {
  const [label, setLabel] = useState('Sign in');
  const [href, setHref] = useState('/account/');
  useEffect(() => {
    const store = browserStorage();
    const signin = new URLSearchParams(window.location.search).get('signin');
    if (signin === 'ok' || signin === 'first') rememberSignedIn(store, true);
    // A first sign-in's mark is the account page's to read and clear.
    if (signin === 'ok') window.history.replaceState(null, '', withoutSignInParam(window.location.pathname, window.location.search));
    setHref(accountHref(window.location.pathname, window.location.search));
    if (!seemsSignedIn(store)) return;
    sharedMe(apiUrl)
      .then((me) => {
        if (me === null) rememberSignedIn(store, false);
        setLabel(navLabel(me));
      })
      .catch(() => undefined);
  }, [apiUrl]);
  return (
    <a href={href} className={current ? 'active' : undefined} aria-current={current ? 'page' : undefined}>
      {label}
    </a>
  );
}
