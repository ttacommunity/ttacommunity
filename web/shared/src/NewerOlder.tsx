/** NewerOlder moves between the pages of a list kept newest first: Newer, where you are, Older; nothing when there is only one page. */
import { count } from './numbers';

interface Props {
  page: number;
  pages: number;
  onPage: (page: number) => void;
}

export default function NewerOlder({ page, pages, onPage }: Props) {
  if (pages <= 1) return null;
  return (
    <div className="newer-older">
      <button type="button" className="btn-antique" disabled={page <= 1} onClick={() => onPage(page - 1)}>
        ‹ Newer
      </button>
      <span>
        Page {count(page)} of {count(pages)}
      </span>
      <button type="button" className="btn-antique" disabled={page >= pages} onClick={() => onPage(page + 1)}>
        Older ›
      </button>
    </div>
  );
}
