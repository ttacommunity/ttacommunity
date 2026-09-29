/** Tex shows a TeX formula, rendered by KaTeX as the site's MDX maths is: <Tex tex="r_s > 0.95" />. */
import { useEffect, useRef } from 'react';
import { renderTex } from '../lib/tex';

export default function Tex({ tex }: { tex: string }) {
  const el = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (el.current) renderTex(el.current, tex);
  }, [tex]);
  return <span ref={el} />;
}
