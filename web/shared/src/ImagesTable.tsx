/** A list of stored images as a table: each one small, its address to copy, its size and when it was uploaded, with any actions after. */
import type { ReactNode } from 'react';
import CopyCode from './CopyCode';
import { sizeText } from './numbers';
import type { StoredImage } from './ratings';
import { formatTime } from './time';

interface Props {
  images: StoredImage[];
  caption: string;
  /** zone is the viewer's time zone, which the upload times are shown in and the column names. */
  zone: string;
  /** actions, when given, fills a last column for each image. */
  actions?: (image: StoredImage) => ReactNode;
  className?: string;
}

export default function ImagesTable({ images, caption, zone, actions, className }: Props) {
  return (
    <div className="table-responsive">
      <table className={`leaderboard-table images-table${className === undefined ? '' : ` ${className}`}`}>
        <caption className="visually-hidden">{caption}</caption>
        <thead>
          <tr>
            <th>Image</th>
            <th>Address</th>
            <th>Size</th>
            <th>Uploaded ({zone})</th>
            {actions !== undefined && <th>Actions</th>}
          </tr>
        </thead>
        <tbody>
          {images.map((image) => (
            <tr key={image.key}>
              <td>
                <a href={image.url} target="_blank" rel="noopener noreferrer">
                  <img className="images-table-thumb" src={image.url} alt={image.key} loading="lazy" />
                </a>
              </td>
              <td>
                <CopyCode code={image.url} what="address" />
              </td>
              <td>{sizeText(image.size)}</td>
              <td>{formatTime(image.uploaded, zone)}</td>
              {actions !== undefined && <td>{actions(image)}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
