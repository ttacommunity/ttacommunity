/** A list of stored images as a table's head and body: each one small, its address to copy, its size and when it was uploaded, with any actions after; the page puts them in its table. */
import type { ReactNode } from 'react';
import CopyCode from './CopyCode';
import { sizeText } from './numbers';
import type { StoredImage } from './ratings';
import { formatTime } from './time';

interface Props {
  images: StoredImage[];
  /** zone is the viewer's time zone, which the upload times are shown in and the column names. */
  zone: string;
  /** actions, when given, fills a last column for each image. */
  actions?: (image: StoredImage) => ReactNode;
}

export default function ImagesRows({ images, zone, actions }: Props) {
  return (
    <>
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
    </>
  );
}
