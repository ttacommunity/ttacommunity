/** A count of items needing attention beside a label: a gold badge, read to a screen reader as ", 3 waiting". */

/** Count shows n, and says what the items are (what) to a screen reader; nothing for 0. */
export default function Count({ n, what }: { n: number; what: string }) {
  if (n === 0) return null;
  return (
    <>
      <span className="count-badge" aria-hidden="true">
        {n}
      </span>
      <span className="visually-hidden">
        , {n} {what}
      </span>
    </>
  );
}
