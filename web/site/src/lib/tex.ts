/** How the site renders TeX in interactive components: KaTeX to MathML, as the MDX content is rendered at build. */
import katex, { type KatexOptions } from 'katex';

/** TEX_OPTIONS match the build's rehype-katex setup: MathML only, so no inline styles, and errors thrown. */
export const TEX_OPTIONS: KatexOptions = { output: 'mathml', throwOnError: true };

/** renderTex draws tex into el with KaTeX (DOM nodes, no HTML strings); TeX it cannot read is shown as text. */
export function renderTex(el: HTMLElement, tex: string): void {
  try {
    katex.render(tex, el, TEX_OPTIONS);
  } catch {
    el.textContent = tex;
  }
}
