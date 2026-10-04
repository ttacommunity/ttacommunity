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

/** circuitCard is a circuit's card (Grand Slams, Ladders: not one tournament, so not edited in admin), from its page's own description. */
export function circuitCard(c: { icon: string; formatBadge: string; summary: string; subtitle: string; frequency: string }): CardView {
  return cardView({ icon: c.icon, tagline: c.formatBadge, summary: c.summary, subtitle: c.subtitle, cadence: c.frequency });
}
