/** The assets bucket's images for anyone, newest first, each with the address to link to. */
import { useEffect, useState } from 'react';
import PageHeader from '@ttac/shared/PageHeader';
import { settled, type Load } from '@ttac/shared/load';
import { assets, type StoredImage } from '@ttac/shared/ratings';
import { viewerZone } from '@ttac/shared/time';
import AssetsTable from './AssetsTable';
import Message from './Message';
import Panel from './Panel';
import Waiting from './Waiting';

/** AssetsPage loads the assets once and lists them, in the viewer's time zone. */
export default function AssetsPage({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load<StoredImage[]>>({ kind: 'loading' });
  useEffect(() => {
    void settled(assets(apiUrl)).then(setLoad);
  }, [apiUrl]);
  return (
    <>
      <PageHeader title="Assets" subtitle="Images the site and the community link to. Copy an address to use one." />
      <Waiting load={load} what="the images" />
      {load.kind === 'ready' &&
        (load.data.length === 0 ? (
          <Message>No images yet.</Message>
        ) : (
          <Panel label="Images">
            <AssetsTable images={load.data} zone={viewerZone()} caption="Every image, newest first, with its address" />
          </Panel>
        ))}
    </>
  );
}
