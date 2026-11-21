/** Images, for admins and organisers: the assets bucket's images, uploaded here and listed with the address to link to, each to delete. */
import { useEffect, useRef, useState } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import ImagesTable from '@ttac/shared/ImagesTable';
import type { StoredImage } from '@ttac/shared/ratings';
import { viewerZone } from '@ttac/shared/time';
import * as req from '../lib/admin-requests';
import { siteUrlFor } from '../lib/admin-tabs';
import { adminRequest } from '../lib/admin-api';
import { loadFailure, outcomeOf, refused, shownAt, type Outcome } from '../lib/outcome';
import Notice from './Notice';
import AdminCard from './AdminCard';

const BUCKET: req.ImageBucket = 'assets';

/** ImagesPanel lists the assets bucket, newest first, uploads an image to it, and deletes one. */
export default function ImagesPanel() {
  const [list, setList] = useState<StoredImage[] | null>(null);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [sending, setSending] = useState(false);
  const [name, setName] = useState('');
  const picker = useRef<HTMLInputElement>(null);
  const dialog = useDialog();
  const zone = viewerZone();

  const load = async () => {
    const r = await adminRequest<StoredImage[]>(`/api/images/${BUCKET}`, {});
    if (r.ok) setList(r.data);
    else setOutcome(loadFailure(r.message, 'images'));
  };
  useEffect(() => {
    void load();
  }, []);

  const upload = async () => {
    const file = picker.current?.files?.[0];
    if (file === undefined) {
      setOutcome(refused('Choose an image first.', 'images'));
      return;
    }
    setSending(true);
    const r = await req.send<StoredImage>(req.uploadImage(BUCKET, file, name));
    setSending(false);
    setOutcome(outcomeOf(r, r.ok ? `Uploaded as ${r.data.key}.` : '', 'images'));
    if (r.ok && picker.current !== null) {
      picker.current.value = '';
      setName('');
    }
    await load();
  };

  const remove = async (image: StoredImage, opener: HTMLElement) => {
    const sure = await dialog.confirm(
      { heading: `Delete ${image.key}?`, text: 'Pages that show it show a broken image instead. Its address is never given to another picture.', action: 'Delete', final: true },
      opener,
    );
    if (!sure) return;
    const r = await req.send(req.deleteImage(BUCKET, image.key));
    setOutcome(outcomeOf(r, `Deleted ${image.key}.`, 'images'));
    await load();
  };

  return (
    <AdminCard id="images-heading" title="Images">
      {dialog.element}
      <p className="muted">
        Pictures the site and Discord can link to, at an address that never changes. A PNG, JPEG, GIF or WebP of 5 MB at most; the same picture is
        stored once. Its address starts with the name you give it, filled in from the file's. Copy an image's address to use it. Everyone can see what is here, at <a href={`${siteUrlFor(window.location.hostname)}/assets/`}>the site's assets page</a>.
      </p>
      <div className="images-upload">
        <label>
          <span className="visually-hidden">Image to upload</span>
          <input
            ref={picker}
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            className="input-antique"
            onChange={(e) => setName(e.currentTarget.files?.[0]?.name ?? '')}
          />
        </label>
        <label>
          Name{' '}
          <input type="text" className="input-antique" value={name} maxLength={255} placeholder="the file's name" onChange={(e) => setName(e.currentTarget.value)} />
        </label>
        <button type="button" className="btn-antique" disabled={sending} onClick={() => void upload()}>
          {sending ? 'Uploading…' : 'Upload'}
        </button>
      </div>
      <Notice outcome={shownAt(outcome, 'images')} onClose={() => setOutcome(null)} />
      {list === null ? null : list.length === 0 ? (
        <p className="muted">No images yet.</p>
      ) : (
        <ImagesTable
          images={list}
          zone={zone}
          caption="Every image, newest first, with its address"
          className="admin-table"
          actions={(image) => (
            <button type="button" className="btn-settings" onClick={(e) => void remove(image, e.currentTarget)}>
              Delete
            </button>
          )}
        />
      )}
    </AdminCard>
  );
}
