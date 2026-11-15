/** A row of buttons choosing one value, in one of the site's three looks: a choice, tabs or a filter. */
import type { ReactNode } from 'react';

export interface Option<T> {
  value: T;
  label: ReactNode;
  /** Shown on hover through the site's tooltip. */
  tooltip?: string;
  title?: string;
}

/** choice: a setting of what is shown (the format bar); tabs: which part of a page (the records tabs); filter: which of many, such as a year (the year pills). */
export type Look = 'choice' | 'tabs' | 'filter';

const GROUP: Record<Look, string> = { choice: 'format-bar-group', tabs: 'ach-tabs', filter: 'year-pills' };
const BUTTON: Record<Look, string> = { choice: 'format-btn', tabs: 'records-tab-btn', filter: 'year-pill' };

interface Props<T> {
  label: string;
  options: Option<T>[];
  value: T;
  look?: Look;
  /** Replaces the group's class, for a page whose group is also its bar. */
  groupClass?: string;
  /** Shown before the buttons, such as "Format:". */
  prefix?: ReactNode;
  /** Each option as a link (a page address), or else as a button calling onChange. */
  link?: (value: T) => { href: string; onClick?: (e: React.MouseEvent<HTMLAnchorElement>) => void };
  onChange?: (value: T) => void;
}

/** Segmented shows each option as a button or link, with the chosen one pressed. */
export default function Segmented<T extends string | number>({ label, options, value, look = 'choice', groupClass, prefix, link, onChange }: Props<T>) {
  return (
    <div role="group" aria-label={label} className={groupClass ?? GROUP[look]}>
      {prefix}
      {options.map((o) => {
        const chosen = o.value === value;
        const className = chosen ? `${BUTTON[look]} active` : BUTTON[look];
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
