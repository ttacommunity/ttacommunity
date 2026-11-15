/** Why a whole page could not load, in a panel of the theme. */
import AdminCard from './AdminCard';

/** PageError shows a page's load failure, with its message, in place of the page. */
export default function PageError({ message }: { message: string }) {
  return (
    <AdminCard id="page-error-heading" title="This page could not load" className="page-error" role="alert">
      <p>{message}</p>
    </AdminCard>
  );
}
