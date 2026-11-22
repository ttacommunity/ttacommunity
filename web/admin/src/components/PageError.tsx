/** Why a whole page could not load, as the site's warning note. */
import Note from '@ttac/shared/Note';

/** PageError shows a page's load failure, with its message, in place of the page, announced to screen readers. */
export default function PageError({ message }: { message: string }) {
  return (
    <div role="alert">
      <Note title="This page could not load" tone="warning">
        <p>{message}</p>
      </Note>
    </div>
  );
}
