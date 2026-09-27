import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import Segmented from './Segmented';

const options = [
  { value: 'a', label: 'Alpha' },
  { value: 'b', label: 'Beta' },
];

describe('Segmented', () => {
  it('renders one button per option inside a labelled group', () => {
    const html = renderToStaticMarkup(<Segmented label="Format" options={options} value="a" onChange={() => {}} />);
    expect(html).toMatch(/^<div role="group" aria-label="Format" class="format-bar-group">/);
    expect(html.match(/<button type="button"/g)).toHaveLength(2);
  });

  it('marks only the chosen option as pressed and active', () => {
    const html = renderToStaticMarkup(<Segmented label="Format" options={options} value="b" onChange={() => {}} />);
    expect(html).toContain('<button type="button" class="format-btn" aria-pressed="false">Alpha</button>');
    expect(html).toContain('<button type="button" class="format-btn active" aria-pressed="true">Beta</button>');
  });
});
