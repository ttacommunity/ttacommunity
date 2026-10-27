/** "Data we keep" on the account page: a button that opens a dialog listing everything kept about the signed-in player, fetched when it opens. */
import { useEffect, useRef, useState } from 'react';
import { errorText, type Load } from '@ttac/shared/load';
import { useModal } from '@ttac/shared/modal';
import { fetchMyData, keptGroups, shownZone, type KeptGroup, type MyData } from '../lib/kept-data';
import Waiting from './Waiting';
import './KeptData.css';

/** KeptData is the button, and the dialog while it is open; savedZone is the player's own zone, for the times. */
export default function KeptData({ apiUrl, savedZone }: { apiUrl: string; savedZone: string }) {
  const [opener, setOpener] = useState<HTMLElement | null>(null);
  return (
    <>
      <button type="button" className="btn-antique" onClick={(e) => setOpener(e.currentTarget)}>
        Data we keep
      </button>
      {opener !== null && <KeptDialog apiUrl={apiUrl} savedZone={savedZone} opener={opener} onClose={() => setOpener(null)} />}
    </>
  );
}

function KeptDialog({ apiUrl, savedZone, opener, onClose }: { apiUrl: string; savedZone: string; opener: HTMLElement; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [load, setLoad] = useState<Load<MyData>>({ kind: 'loading' });
  const zone = shownZone(savedZone, Intl.DateTimeFormat().resolvedOptions().timeZone);
  // A click on the backdrop closes it; Esc and Close are the keyboard's ways.
  const closed = useModal(dialog, opener, onClose, { backdropCloses: true });

  useEffect(() => {
    let stale = false;
    fetchMyData(apiUrl)
      .then((data) => {
        if (stale) return;
        setLoad(data === null ? { kind: 'error', message: 'You are signed out. Sign in again to see what we keep.' } : { kind: 'ready', data });
      })
      .catch((e: unknown) => !stale && setLoad({ kind: 'error', message: errorText(e) }));
    return () => {
      stale = true;
    };
  }, [apiUrl]);

  const names = typeof Intl.DisplayNames === 'function' ? new Intl.DisplayNames(['en'], { type: 'region' }) : null;
  const groups = load.kind === 'ready' ? keptGroups(load.data, zone, (code) => names?.of(code) ?? code) : [];
  return (
    <dialog ref={dialog} className="kept-modal" aria-labelledby="kept-title" onClose={closed}>
      <div className="kept-head">
        <h2 id="kept-title">Data we keep</h2>
        <button type="button" className="kept-close" aria-label="Close" onClick={() => dialog.current?.close()}>
          ×
        </button>
      </div>
      <div className="kept-body">
        <p className="kept-intro">
          Everything we keep about you, and when each is removed. Times are in {zone}. To have something corrected or removed sooner, write to
          privacy@ttacommunity.org.
        </p>
        <Waiting load={load} what="what we keep" form="inline" />
        {groups.map((g) => (
          <Group key={g.id} group={g} />
        ))}
      </div>
    </dialog>
  );
}

function Group({ group }: { group: KeptGroup }) {
  return (
    <section className="kept-group" aria-labelledby={`kept-${group.id}`}>
      <h3 id={`kept-${group.id}`}>{group.heading}</h3>
      {group.rule !== '' && <p className="kept-rule">{group.rule}</p>}
      {group.lists.length === 0 && <p className="kept-rule">{group.empty}</p>}
      {group.lists.map((l) => (
        <div key={l.id} className={l.thisBrowser ? 'kept-list kept-this' : 'kept-list'}>
          {l.title !== '' && <h4>{l.title}</h4>}
          {l.rule !== '' && <p className="kept-rule">{l.rule}</p>}
          <dl>
            {l.facts.map((f) => (
              <div key={f.key} className="kept-fact">
                <dt>{f.label}</dt>
                <dd>
                  {f.value}
                  {f.note !== '' && <span className="kept-note">{f.note}</span>}
                </dd>
              </div>
            ))}
          </dl>
        </div>
      ))}
    </section>
  );
}
