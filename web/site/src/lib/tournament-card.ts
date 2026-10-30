/** How a tournament presents itself, the same on its card and its page's header. */

/** A card as admin writes it: an empty field is unset. */
export type Card = { icon: string; tagline: string; summary: string; subtitle: string; cadence: string };

/** CardView is a card ready to show: always an icon, null for a description or information box not written. */
export type CardView = { icon: string; tagline: string | null; summary: string | null; subtitle: string | null; cadence: string | null };

/** cardView fills in the default icon and drops empty fields, for a card or none (still loading). */
export function cardView(card: Card | undefined): CardView {
  return {
    icon: card?.icon || '📜',
    tagline: card?.tagline || null,
    summary: card?.summary || null,
    subtitle: card?.subtitle || null,
    cadence: card?.cadence || null,
  };
}
