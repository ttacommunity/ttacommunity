/** A row of buttons choosing one value, in one of the site's three looks: a choice, tabs or a filter. */
import type { ReactNode } from 'react';
import { buttonClass, groupClass, type Look } from './segmented-look';

export interface Option<T> {
  value: T;
  label: ReactNode;
  /** Shown on hover through the site's tooltip. */
  tooltip?: string;
  title?: string;
}

export type { Look };

interface Props<T> {
  label: string;
  options: Option<T>[];
  value: T;
  look?: Look;
  /** One option on each line, for a narrow column such as the options drawer. */
  stacked?: boolean;
  /** Shown before the buttons, such as "Format:". */
  prefix?: ReactNode;
  /** Each option as a link (a page address), or else as a button calling onChange. */
  link?: (value: T) => { href: string; onClick?: (e: React.MouseEvent<HTMLAnchorElement>) => void };
  onChange?: (value: T) => void;
}

/** Segmented shows each option as a button or link, with the chosen one pressed. */
export default function Segmented<T extends string | number>({ label, options, value, look = 'choice', stacked = false, prefix, link, onChange }: Props<T>) {
  return (
    <div role="group" aria-label={label} className={groupClass(look, stacked)}>
      {prefix}
      {options.map((o) => {
        const chosen = o.value === value;
        const className = buttonClass(look, chosen);
        return link !== undefined ? (
          <a key={o.value} {...link(o.value)} className={className} aria-current={chosen ? 'true' : undefined} data-tooltip={o.tooltip} title={o.title}>
            {o.label}
          </a>
        ) : (
          <button
            key={o.value}
            type="button"
            className={className}
            aria-pressed={chosen}
            data-tooltip={o.tooltip}
            title={o.title}
            onClick={() => onChange?.(o.value)}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
