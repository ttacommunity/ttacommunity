/** Why a whole page could not load, in a panel of the theme. */

/** PageError shows a page's load failure, with its message, in place of the page. */
export default function PageError({ message }: { message: string }) {
  return (
    <section className="rating-card admin-card page-error" role="alert" aria-labelledby="page-error-heading">
      <h2 id="page-error-heading" className="rating-card-heading">
        This page could not load
      </h2>
      <p>{message}</p>
    </section>
  );
}
