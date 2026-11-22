/** The site's table of stored images: the shared rows in a table that scrolls sideways on a phone. */
import ImagesRows from '@ttac/shared/ImagesRows';
import type { StoredImage } from '@ttac/shared/ratings';
import ScrollTable from './ScrollTable';

/** AssetsTable lists images, newest first as given, with their upload times in zone. */
export default function AssetsTable({ images, zone, caption }: { images: StoredImage[]; zone: string; caption: string }) {
  return (
    <ScrollTable label="Images">
      <table className="leaderboard-table">
        <caption className="visually-hidden">{caption}</caption>
        <ImagesRows images={images} zone={zone} />
      </table>
    </ScrollTable>
  );
}
