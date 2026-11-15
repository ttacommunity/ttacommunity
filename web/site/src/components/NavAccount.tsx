/** The nav's account link: Sign in, or who is signed in, leading to the account page with the way back to this one, and for admins and organisers a link to admin. It asks the api only in a browser that has signed in. */
import { useEffect, useState } from 'react';
import { accountHref, navLabel, rememberSignedIn, seemsSignedIn, sharedMe, showsAdminLink, withoutSignInParam } from '../lib/account';
import { browserStorage } from '@ttac/shared/storage';

/** NavAccount marks a sign-in that just finished, then names whoever is signed in. */
export default function NavAccount({ apiUrl, adminUrl, current }: { apiUrl: string; adminUrl: string; current: boolean }) {
  const [label, setLabel] = useState('Sign in');
  const [admin, setAdmin] = useState(false);
  const [href, setHref] = useState('/account/');
  useEffect(() => {
    const store = browserStorage();
    const signin = new URLSearchParams(window.location.search).get('signin');
    if (signin === 'ok' || signin === 'first') rememberSignedIn(store, true);
    // A first sign-in's mark is the account page's to read and clear.
    if (signin === 'ok') window.history.replaceState(null, '', withoutSignInParam(window.location.pathname, window.location.search));
    setHref(accountHref(window.location.pathname, window.location.search));
    const updateMoviesBadge = (count: number) => {
      for (const el of document.querySelectorAll<HTMLElement>('[data-movies-badge]')) {
        if (count > 0) {
          el.textContent = count > 99 ? '99+' : String(count);
          el.hidden = false;
        } else {
          el.textContent = '';
          el.hidden = true;
        }
      }
    };
    const onMoviesSeen = () => updateMoviesBadge(0);
    window.addEventListener('ttac:movies-seen', onMoviesSeen);
    if (!seemsSignedIn(store)) return () => window.removeEventListener('ttac:movies-seen', onMoviesSeen);
    sharedMe(apiUrl)
      .then((me) => {
        if (me === null) rememberSignedIn(store, false);
        setLabel(navLabel(me));
        setAdmin(showsAdminLink(me));
        updateMoviesBadge(me?.unseen_movies ?? 0);
      })
      .catch(() => undefined);
    return () => window.removeEventListener('ttac:movies-seen', onMoviesSeen);
  }, [apiUrl]);
  return (
    <>
      <a href={href} className={current ? 'active' : undefined} aria-current={current ? 'page' : undefined}>
        {label}
      </a>
      {admin && <a href={adminUrl}>Admin</a>}
    </>
  );
}
