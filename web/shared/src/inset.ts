/** An Inset's forms, and the classes that draw them. */

/** block holds any content; figures lays its children out in a row; line is one short muted line; row puts what it is on the left and its figures or actions on the right. */
export type InsetForm = 'block' | 'figures' | 'line' | 'row';

/** insetClass returns the box's classes for the form asked for. */
export function insetClass(form: InsetForm): string {
  return form === 'block' ? 'inset' : `inset inset-${form}`;
}
