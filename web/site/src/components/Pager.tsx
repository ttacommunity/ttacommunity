/** Paging through a long table, as upstream's leaderboard does: where you are, First/Prev, a page box, Next/Last. */
import { useEffect, useState } from 'react';
import { typedPage } from '../lib/pager';
import './Pager.css';

interface Props {
  page: number;
  pages: number;
  /** The rows in all pages and what they are, such as 1,402 "players"; left out when not known. */
  total?: { count: number; noun: string };
  /** The link to a page: its address, and a handler for pages that change without reloading. */
  link: (page: number) => { href: string; onClick?: (e: React.MouseEvent<HTMLAnchorElement>) => void };
  /** Goes to a page typed in the box. */
  go: (page: number) => void;
}

/** Pager shows nothing for a single page. */
export default function Pager({ page, pages, total, link, go }: Props) {
  const [typed, setTyped] = useState(String(page));
  useEffect(() => setTyped(String(page)), [page]);
  if (pages <= 1) return null;
  return (
    <nav className="pagination" aria-label="Pages">
      <div>
        Showing page <strong>{page}</strong> of <strong>{pages}</strong>
        {total !== undefined && ` (${total.count.toLocaleString('en')} ${total.noun})`}
      </div>
      <div className="pagination-links">
        {page > 1 && (
          <>
            <a {...link(1)} title="Jump to First Page">
              &laquo; First
            </a>
            <a {...link(page - 1)} title="Previous Page">
              &lsaquo; Prev
            </a>
          </>
        )}
        <form
          className="page-jumper"
          onSubmit={(e) => {
            e.preventDefault();
            go(typedPage(typed, pages));
          }}
        >
          <span>Page</span>
          <input type="number" min={1} max={pages} aria-label="Page number" enterKeyHint="go" value={typed} onChange={(e) => setTyped(e.target.value)} />
          <span>/ {pages}</span>
          <button type="submit" title="Go to page">
            Go
          </button>
        </form>
        {page < pages && (
          <>
            <a {...link(page + 1)} title="Next Page">
              Next &rsaquo;
            </a>
            <a {...link(pages)} title="Jump to Last Page">
              Last &raquo;
            </a>
          </>
        )}
      </div>
    </nav>
  );
}
