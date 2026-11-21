/** A player's featured accolades, each a highlight, on the career box and the achievements page. */
import Highlight from './Highlight';
import './Honours.css';

/** Honours lists each accolade with a star, in a grid. */
export default function Honours({ accolades }: { accolades: readonly string[] }) {
  return (
    <ul className="honours">
      {accolades.map((a) => (
        <li key={a}>
          <Highlight>
            <span className="honour">
              <span className="honour-star" aria-hidden="true">
                ★
              </span>
              {a}
            </span>
          </Highlight>
        </li>
      ))}
    </ul>
  );
}
